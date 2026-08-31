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
	"maps"
	"slices"
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
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/storageclass"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	servicestat "github.com/deckhouse/virtualization-controller/pkg/controller/service/stat"
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
	CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage) (requeue bool, reason string, err error)
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

func (s Sources) CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage) (requeue bool, reason string, err error) {
	var reasons []string

	// Iterate over the data sources in a stable order: the reasons are merged into the
	// Terminating condition message, so a map iteration would reshuffle them (and thus
	// the part of the message that survives truncation) on every reconcile.
	for _, dsType := range slices.Sorted(maps.Keys(s.sources)) {
		source := s.sources[dsType]

		sourceRequeue, sourceReason, err := source.CleanUp(ctx, vi)
		if err != nil {
			return false, "", err
		}

		requeue = requeue || sourceRequeue
		reasons = append(reasons, sourceReason)
	}

	reason = service.MergeCleanUpReasons(reasons...)
	if requeue && reason == "" {
		reason = service.DefaultCleanUpReason
	}

	return requeue, reason, nil
}

type Cleaner interface {
	CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage) (requeue bool, reason string, err error)
	CleanUpSupplements(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error)
}

func CleanUp(ctx context.Context, vi *v1alpha2.VirtualImage, c Cleaner) (requeue bool, reason string, err error) {
	if object.ShouldCleanupSubResources(vi) {
		return c.CleanUp(ctx, vi)
	}

	return false, "", nil
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
// DVCR phase). The previous progress is kept untouched when the statSvc service,
// the pod, or the metric is not yet available.
func refreshPVCImportProgress(
	vi *v1alpha2.VirtualImage,
	statSvc Stat,
	pod *corev1.Pod,
	scale *servicestat.ScaleOption,
) {
	if statSvc == nil || pod == nil {
		return
	}

	var opts []servicestat.GetProgressOption
	if scale != nil {
		opts = append(opts, scale)
	}
	vi.Status.Progress = servicestat.CapProgressBelow(statSvc.GetProgress(vi.GetUID(), pod, vi.Status.Progress, opts...), 100)
}

func pvcImporterPodPhase(pod *corev1.Pod) corev1.PodPhase {
	if pod == nil || pod.Status.Phase == "" {
		return corev1.PodPending
	}
	return pod.Status.Phase
}

// pvcImportFailed sets the failed phase and condition when the pvc-importer cannot finish, and
// reports whether it did. The pod runs with RestartPolicy: OnFailure and practically never
// reaches PodFailed, so a permanently broken import is recognized by the reason the importer
// leaves in its termination message; without that the image reports "importing" forever.
//
// The caller must keep requeueing afterwards. The importer pod belongs to the PersistentVolumeClaim,
// not to the image, and the image only watches pods it owns: nothing else brings the reconcile back
// while the pod keeps retrying, and the failure is not always final - the condition behind it can be
// fixed from the outside.
func pvcImportFailed(vi *v1alpha2.VirtualImage, cb *conditions.ConditionBuilder, pod *corev1.Pod) bool {
	failure := monitoring.PermanentFailureFromPod(pod)
	if failure == "" {
		if pvcImporterPodPhase(pod) != corev1.PodFailed {
			return false
		}
		failure = "VirtualImage importer Pod failed"
	}

	vi.Status.Phase = v1alpha2.ImageFailed
	cb.Status(metav1.ConditionFalse).Reason(vicondition.ProvisioningFailed).Message(service.CapitalizeFirstLetter(failure) + ".")
	return true
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
			Message(fmt.Sprintf("The underlying PersistentVolumeClaim %q was not found.", supgen.PersistentVolumeClaim().String()))
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
	case errors.Is(err, servicestat.ErrNotInitialized), errors.Is(err, servicestat.ErrNotScheduled):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningNotStarted).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return nil
	case errors.Is(err, servicestat.ErrProvisioningFailed):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return nil
	default:
		return err
	}
}

func setPhaseConditionFromStorageError(err error, vi *v1alpha2.VirtualImage, cb *conditions.ConditionBuilder) (reconcile.Result, bool, error) {
	switch {
	case err == nil:
		return reconcile.Result{}, false, nil
	case errors.Is(err, volumemode.ErrStorageProfileNotFound):
		vi.Status.Phase = v1alpha2.ImageFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("The StorageClass is not fully configured in the cluster. Check the StorageClass name or set a default StorageClass.")
		return reconcile.Result{}, true, nil
	case errors.Is(err, service.ErrDefaultStorageClassNotFound):
		vi.Status.Phase = v1alpha2.ImagePending
		vi.Status.Progress = ""
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass.")
		return reconcile.Result{}, true, nil
	case common.ErrQuotaExceeded(err):
		// The result carries the retry the quota branch asks for: dropping it here left that
		// retry dead and the image waiting for a reconcile that never came.
		return setQuotaExceededPhaseCondition(cb, &vi.Status.Phase, err, vi.CreationTimestamp), true, nil
	default:
		return reconcile.Result{}, false, err
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
	statSvc Stat,
	disk *service.DiskService,
) (reconcile.Result, error) {
	if pvc == nil {
		if err := statSvc.CheckPod(pod); err != nil {
			vi.Status.Phase = v1alpha2.ImageFailed
			switch {
			case errors.Is(err, servicestat.ErrProvisioningFailed):
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
		vi.Status.DownloadSpeed = statSvc.GetDownloadSpeed(vi.GetUID(), pod)

		diskSize, err := getPVCSizeFromPod(statSvc, pod)
		if err != nil {
			setPhaseConditionToFailed(cb, &vi.Status.Phase, err)
			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}

		err = createPVCImportTarget(ctx, vi, supgen, diskSize, source, disk)
		if res, updated, err := setPhaseConditionFromStorageError(err, vi, cb); err != nil || updated {
			return res, err
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
			Message("Preparing the PersistentVolumeClaim for the image.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	vi.Status.Target.PersistentVolumeClaim = pvc.Name
	if pvc.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		vi.Status.Phase = v1alpha2.ImageReady
		cb.Status(metav1.ConditionTrue).Reason(vicondition.Ready).Message("")
		vi.Status.Progress = "100%"
		vi.Status.Size = statSvc.GetSize(pod)
		vi.Status.CDROM = statSvc.GetCDROM(pod)
		vi.Status.Format = imageformat.StorageFormat(pvc)
		vi.Status.DownloadSpeed = statSvc.GetDownloadSpeed(vi.GetUID(), pod)
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	// The pod is read once per reconcile: the phase, the failure report and the progress
	// metric must all describe the same attempt.
	importPod, err := disk.GetPVCImporterPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc-importer pod: %w", err)
	}
	if pvcImportFailed(vi, cb, importPod) {
		return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
	}

	vi.Status.Phase = v1alpha2.ImageProvisioning
	cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("Importing data into the PersistentVolumeClaim.")
	if vi.Status.Progress == "" {
		vi.Status.Progress = "50.0%"
	}
	if pvcImporterPodPhase(importPod) == corev1.PodSucceeded {
		return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
	}
	// The DVCR phase fills the first half of the overall progress, so the
	// pvc-importer metric (0..100) is projected into the 50..100 slice.
	refreshPVCImportProgress(vi, statSvc, importPod, servicestat.NewScaleOption(50, 100))
	return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
}

func getPVCSizeFromPod(statSvc Stat, pod *corev1.Pod) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(statSvc.GetSize(pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", statSvc.GetSize(pod).UnpackedBytes, err)
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
	statSvc Stat,
	disk *service.DiskService,
	ready func(),
) (reconcile.Result, error) {
	if pvc == nil {
		// The admission webhook rejects cross-CSI PVC sources only when the source
		// provisioner is determinable at creation time; re-check here so a source
		// that became Ready on a different CSI driver afterwards fails with a clear
		// message instead of starting a copy that can never succeed.
		if source != nil && source.PVC != nil {
			err := validatePVCSourceProvisionerCompatibility(ctx, disk.Client(), vi.Status.StorageClassName, source.PVC)
			if err != nil {
				vi.Status.Phase = v1alpha2.ImageFailed
				cb.
					Status(metav1.ConditionFalse).
					Reason(vicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error()))
				return reconcile.Result{}, nil
			}
		}

		err := createPVCImportTarget(ctx, vi, supgen, size, source, disk)
		if res, updated, err := setPhaseConditionFromStorageError(err, vi, cb); err != nil || updated {
			return res, err
		}

		target, err := disk.GetPersistentVolumeClaim(ctx, supgen)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("fetch target pvc: %w", err)
		}
		if target == nil {
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}
		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("Preparing the PersistentVolumeClaim for the image.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	vi.Status.Target.PersistentVolumeClaim = pvc.Name
	if pvc.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		vi.Status.Phase = v1alpha2.ImageReady
		cb.Status(metav1.ConditionTrue).Reason(vicondition.Ready).Message("")
		vi.Status.Progress = "100%"
		ready()
		vi.Status.Format = imageformat.StorageFormat(pvc)
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	importPod, err := disk.GetPVCImporterPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc-importer pod: %w", err)
	}
	if pvcImportFailed(vi, cb, importPod) {
		return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
	}

	vi.Status.Phase = v1alpha2.ImageProvisioning
	if vi.Status.Progress == "" {
		vi.Status.Progress = "0%"
	}
	cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("Importing data into the PersistentVolumeClaim.")
	if pvcImporterPodPhase(importPod) == corev1.PodSucceeded {
		return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
	}
	refreshPVCImportProgress(vi, statSvc, importPod, nil)
	return reconcile.Result{RequeueAfter: pvcImportProgressRequeue}, nil
}

// validatePVCSourceProvisionerCompatibility forbids provisioning from a source PVC
// whose storage class is backed by a different CSI driver than the target storage
// class: a PVC-to-PVC copy cannot cross the driver boundary. The check is skipped
// when either provisioner cannot be determined.
func validatePVCSourceProvisionerCompatibility(ctx context.Context, c client.Client, targetSCName string, sourcePVC *service.PVCImportSourcePVC) error {
	sourceClaim, err := object.FetchObject(ctx, types.NamespacedName{Name: sourcePVC.Name, Namespace: sourcePVC.Namespace}, c, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return fmt.Errorf("fetch source pvc: %w", err)
	}
	if sourceClaim == nil || sourceClaim.Spec.StorageClassName == nil || *sourceClaim.Spec.StorageClassName == "" {
		return nil
	}

	sourceProvisioner, err := storageclass.ProvisionerOf(ctx, c, *sourceClaim.Spec.StorageClassName)
	if err != nil || sourceProvisioner == "" {
		return nil
	}

	targetProvisioner, err := storageclass.ProvisionerOf(ctx, c, targetSCName)
	if err != nil || targetProvisioner == "" {
		return nil
	}

	if sourceProvisioner != targetProvisioner {
		return fmt.Errorf(
			"cannot provision to storage class %q: incompatible storage providers. "+
				"Source is backed by %q, target storage class uses %q. "+
				"Cross-provider PVC copy is not supported",
			targetSCName, sourceProvisioner, targetProvisioner,
		)
	}

	return nil
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
		// The API declares this reason for exactly this case; the code used to report a
		// generic failure instead, so nothing could tell a quota apart from any other one.
		Reason(vicondition.QuotaExceeded)

	if creationTimestamp.Add(30 * time.Minute).After(time.Now()) {
		cb.Message(fmt.Sprintf("Quota exceeded: %s; Please configure quotas or try recreating the resource later.", err))
		return reconcile.Result{}
	}

	cb.Message(fmt.Sprintf("Quota exceeded: %s; Retry in %d minute.", err, retryPeriod))
	return reconcile.Result{RequeueAfter: retryPeriod * time.Minute}
}
