/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package source

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

// needBounderForClone reports whether a bounder pod must be created to unblock
// provisioning of the target PVC.
//
// A smart-clone target (CSI clone / VolumeSnapshot restore) is dynamically
// provisioned from a dataSource and has no importer pod. On a
// WaitForFirstConsumer storage class such a PVC stays Pending until a consumer
// pod is scheduled. A VirtualImage on PVC never gets a VirtualMachine consumer,
// so without a bounder pod (whose only job is to get scheduled and trigger the
// binding) the import would hang forever. Host-assisted imports bind the target
// via the prime-PVC rebind and never need a bounder.
func needBounderForClone(ctx context.Context, cl client.Client, pvc *corev1.PersistentVolumeClaim) (bool, error) {
	if pvc == nil || pvc.Status.Phase == corev1.ClaimBound {
		return false, nil
	}
	if !service.IsSmartClonePVC(pvc) {
		return false, nil
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		return false, nil
	}

	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: *pvc.Spec.StorageClassName}, cl, &storagev1.StorageClass{})
	if err != nil {
		return false, fmt.Errorf("fetch storage class: %w", err)
	}
	if sc == nil || sc.VolumeBindingMode == nil || *sc.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer {
		return false, nil
	}

	return true, nil
}

type Handler interface {
	StoreToDVCR(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error)
	StoreToPVC(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error)
	CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage) (bool, error)
	Validate(ctx context.Context, vi *v1alpha2.VirtualImage) error
}

type Sources struct {
	sources map[v1alpha2.DataSourceType]Handler
}

func NewSources() *Sources {
	return &Sources{
		sources: make(map[v1alpha2.DataSourceType]Handler),
	}
}

func (s Sources) Set(dsType v1alpha2.DataSourceType, h Handler) {
	s.sources[dsType] = h
}

func (s Sources) For(dsType v1alpha2.DataSourceType) (Handler, bool) {
	source, ok := s.sources[dsType]
	return source, ok
}

func (s Sources) Changed(_ context.Context, vi *v1alpha2.VirtualImage) bool {
	return vi.Generation != vi.Status.ObservedGeneration
}

func (s Sources) CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage) (bool, error) {
	var requeue bool

	for _, source := range s.sources {
		ok, err := source.CleanUp(ctx, vi)
		if err != nil {
			return false, err
		}

		requeue = requeue || ok
	}

	return requeue, nil
}

type Cleaner interface {
	CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage) (bool, error)
	CleanUpSupplements(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error)
}

func CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage, c Cleaner) (bool, error) {
	if object.ShouldCleanupSubResources(vi) {
		return c.CleanUp(ctx, vi)
	}

	return false, nil
}

func CleanUpSupplements(ctx context.Context, vi *v1alpha2.VirtualImage, c Cleaner) (reconcile.Result, error) {
	if object.ShouldCleanupSubResources(vi) {
		return c.CleanUpSupplements(ctx, vi)
	}

	return reconcile.Result{}, nil
}

func IsImageProvisioningFinished(c metav1.Condition) bool {
	return c.Reason == vicondition.Ready.String()
}

// pvcImportProgressRequeue is how often a VirtualImage on PVC is requeued while
// its import is in flight so that vi.Status.Progress is refreshed from the
// pvc-importer pod metric and streams intermediate percentages.
const pvcImportProgressRequeue = 2 * time.Second

// refreshPVCImportProgress republishes vi.Status.Progress from the pvc-importer
// pod's kubevirt_cdi_import_progress_total metric while the import to the target
// PVC is running, so the reported progress streams intermediate percentages
// instead of jumping straight from its starting value to 100%.
//
// When scale is set, the metric (0..100) is projected into the
// [scale.Low, scale.High] slice of the overall progress (e.g. 50..100 for the
// HTTP/Registry/Upload data sources whose first half is already filled by the
// DVCR phase). The previous progress is kept untouched when the stat service,
// the pod, or the metric is not yet available.
func refreshPVCImportProgress(
	ctx context.Context,
	vi *v1alpha2.VirtualImage,
	disk *service.DiskService,
	stat Stat,
	supgen supplements.Generator,
	scale *service.ScaleOption,
) error {
	if stat == nil {
		return nil
	}

	pod, err := disk.GetPVCImporterPod(ctx, supgen)
	if err != nil {
		return fmt.Errorf("fetch pvc-importer pod: %w", err)
	}
	if pod == nil {
		return nil
	}

	var opts []service.GetProgressOption
	if scale != nil {
		opts = append(opts, scale)
	}
	vi.Status.Progress = service.CapProgressBelow(stat.GetProgress(vi.GetUID(), pod, vi.Status.Progress, opts...), 100)
	return nil
}

func pvcImporterPodPhase(ctx context.Context, disk *service.DiskService, supgen supplements.Generator) (corev1.PodPhase, error) {
	pod, err := disk.GetPVCImporterPod(ctx, supgen)
	if err != nil {
		return "", fmt.Errorf("fetch pvc-importer pod: %w", err)
	}
	if pod == nil || pod.Status.Phase == "" {
		return corev1.PodPending, nil
	}
	return pod.Status.Phase, nil
}

func setPhaseConditionForFinishedImage(
	pvc *corev1.PersistentVolumeClaim,
	cb *conditions.ConditionBuilder,
	phase *v1alpha2.ImagePhase,
	supgen supplements.Generator,
) {
	switch pvc {
	case nil:
		*phase = v1alpha2.ImagePVCLost
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.PVCLost).
			Message(fmt.Sprintf("PVC %s not found.", supgen.PersistentVolumeClaim().String()))
	default:
		*phase = v1alpha2.ImageReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.Ready).
			Message("")
	}
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *v1alpha2.ImagePhase, err error) {
	*phase = v1alpha2.ImageFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vicondition.ProvisioningFailed).
		Message(service.CapitalizeFirstLetter(err.Error()))
}

func setPhaseConditionFromPodError(cb *conditions.ConditionBuilder, vi *v1alpha2.VirtualImage, err error) error {
	vi.Status.Phase = v1alpha2.ImageFailed

	switch {
	case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningNotStarted).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return nil
	case errors.Is(err, service.ErrProvisioningFailed):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return nil
	default:
		return err
	}
}

func setPhaseConditionFromStorageError(err error, vi *v1alpha2.VirtualImage, cb *conditions.ConditionBuilder) (bool, error) {
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, volumemode.ErrStorageProfileNotFound):
		vi.Status.Phase = v1alpha2.ImageFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("StorageProfile not found in the cluster: Please check a StorageClass name in the cluster or set a default StorageClass.")
		return true, nil
	case errors.Is(err, service.ErrDefaultStorageClassNotFound):
		vi.Status.Phase = v1alpha2.ImagePending
		vi.Status.Progress = ""
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass.")
		return true, nil
	case common.ErrQuotaExceeded(err):
		_ = setQuotaExceededPhaseCondition(cb, &vi.Status.Phase, err, vi.CreationTimestamp)
		return true, nil
	default:
		return false, err
	}
}

func reconcilePVCImportFromDVCR(
	ctx context.Context,
	vi *v1alpha2.VirtualImage,
	pod *corev1.Pod,
	pvc *corev1.PersistentVolumeClaim,
	source *service.PVCImportSource,
	cb *conditions.ConditionBuilder,
	supgen supplements.Generator,
	stat Stat,
	disk *service.DiskService,
) (reconcile.Result, error) {
	if pvc == nil {
		if err := stat.CheckPod(pod); err != nil {
			vi.Status.Phase = v1alpha2.ImageFailed
			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		vi.Status.Progress = "50.0%"
		vi.Status.DownloadSpeed = stat.GetDownloadSpeed(vi.GetUID(), pod)

		diskSize, err := getPVCSizeFromPod(stat, pod)
		if err != nil {
			setPhaseConditionToFailed(cb, &vi.Status.Phase, err)
			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}

		err = createPVCImportTarget(ctx, vi, supgen, diskSize, source, disk)
		if updated, err := setPhaseConditionFromStorageError(err, vi, cb); err != nil || updated {
			return reconcile.Result{}, err
		}

		target, err := disk.GetPersistentVolumeClaim(ctx, supgen)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("fetch target pvc: %w", err)
		}
		if target == nil {
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}
		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC Provisioner not found: create the new one.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	vi.Status.Target.PersistentVolumeClaim = pvc.Name
	if pvc.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		vi.Status.Phase = v1alpha2.ImageReady
		cb.Status(metav1.ConditionTrue).Reason(vicondition.Ready).Message("")
		vi.Status.Progress = "100%"
		vi.Status.Size = stat.GetSize(pod)
		vi.Status.CDROM = stat.GetCDROM(pod)
		vi.Status.DownloadSpeed = stat.GetDownloadSpeed(vi.GetUID(), pod)
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	importPhase, err := pvcImporterPodPhase(ctx, disk, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	if importPhase == corev1.PodFailed {
		vi.Status.Phase = v1alpha2.ImageFailed
		cb.Status(metav1.ConditionFalse).Reason(vicondition.ProvisioningFailed).Message("VirtualImage importer Pod failed.")
		return reconcile.Result{}, nil
	}

	vi.Status.Phase = v1alpha2.ImageProvisioning
	cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("Import is in the process of provisioning to PVC.")
	if vi.Status.Progress == "" {
		vi.Status.Progress = "50.0%"
	}
	if importPhase == corev1.PodSucceeded {
		return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
	}
	// The DVCR phase fills the first half of the overall progress, so the
	// pvc-importer metric (0..100) is projected into the 50..100 slice.
	if err := refreshPVCImportProgress(ctx, vi, disk, stat, supgen, service.NewScaleOption(50, 100)); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
}

func getPVCSizeFromPod(stat Stat, pod *corev1.Pod) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(stat.GetSize(pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", stat.GetSize(pod).UnpackedBytes, err)
	}
	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}
	return service.GetValidatedPVCSize(nil, unpackedSize)
}

func reconcilePVCImportFromReadySource(
	ctx context.Context,
	vi *v1alpha2.VirtualImage,
	pvc *corev1.PersistentVolumeClaim,
	source *service.PVCImportSource,
	size resource.Quantity,
	cb *conditions.ConditionBuilder,
	supgen supplements.Generator,
	stat Stat,
	disk *service.DiskService,
	ready func(),
) (reconcile.Result, error) {
	if pvc == nil {
		err := createPVCImportTarget(ctx, vi, supgen, size, source, disk)
		if updated, err := setPhaseConditionFromStorageError(err, vi, cb); err != nil || updated {
			return reconcile.Result{}, err
		}

		target, err := disk.GetPersistentVolumeClaim(ctx, supgen)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("fetch target pvc: %w", err)
		}
		if target == nil {
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}
		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("PVC Provisioner not found: create the new one.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	vi.Status.Target.PersistentVolumeClaim = pvc.Name
	if pvc.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		vi.Status.Phase = v1alpha2.ImageReady
		cb.Status(metav1.ConditionTrue).Reason(vicondition.Ready).Message("")
		vi.Status.Progress = "100%"
		ready()
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	importPhase, err := pvcImporterPodPhase(ctx, disk, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	if importPhase == corev1.PodFailed {
		vi.Status.Phase = v1alpha2.ImageFailed
		cb.Status(metav1.ConditionFalse).Reason(vicondition.ProvisioningFailed).Message("VirtualImage importer Pod failed.")
		return reconcile.Result{}, nil
	}

	vi.Status.Phase = v1alpha2.ImageProvisioning
	if vi.Status.Progress == "" {
		vi.Status.Progress = "0%"
	}
	cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("Import is in the process of provisioning to PVC.")
	if importPhase == corev1.PodSucceeded {
		return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
	}
	if err := refreshPVCImportProgress(ctx, vi, disk, stat, supgen, nil); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
}

func createPVCImportTarget(
	ctx context.Context,
	vi *v1alpha2.VirtualImage,
	supgen supplements.Generator,
	size resource.Quantity,
	source *service.PVCImportSource,
	disk *service.DiskService,
) error {
	key := supgen.PersistentVolumeClaim()
	switch {
	case source == nil:
		_, err := disk.PersistentVolumeClaim().CreateBlankTarget(ctx, key, vi.Status.StorageClassName, &size, vi, disk, nil)
		return err
	case source.Registry != nil:
		_, err := disk.PersistentVolumeClaim().CreateTargetFromDVCR(ctx, key, vi.Status.StorageClassName, &size, vi, source.Registry, disk, nil)
		return err
	case source.PVC != nil:
		sourceClaim, err := object.FetchObject(ctx, types.NamespacedName{Name: source.PVC.Name, Namespace: source.PVC.Namespace}, disk.Client(), &corev1.PersistentVolumeClaim{})
		if err != nil {
			return fmt.Errorf("fetch source pvc: %w", err)
		}
		if sourceClaim == nil {
			return fmt.Errorf("source pvc %s/%s not found", source.PVC.Namespace, source.PVC.Name)
		}
		_, err = disk.PersistentVolumeClaim().CreateTargetFromPVC(ctx, key, vi.Status.StorageClassName, &size, vi, sourceClaim, disk, nil)
		return err
	default:
		return nil
	}
}

const retryPeriod = 1

func setQuotaExceededPhaseCondition(cb *conditions.ConditionBuilder, phase *v1alpha2.ImagePhase, err error, creationTimestamp metav1.Time) reconcile.Result {
	*phase = v1alpha2.ImageFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vicondition.ProvisioningFailed)

	if creationTimestamp.Add(30 * time.Minute).After(time.Now()) {
		cb.Message(fmt.Sprintf("Quota exceeded: %s; Please configure quotas or try recreating the resource later.", err))
		return reconcile.Result{}
	}

	cb.Message(fmt.Sprintf("Quota exceeded: %s; Retry in %d minute.", err, retryPeriod))
	return reconcile.Result{RequeueAfter: retryPeriod * time.Minute}
}
