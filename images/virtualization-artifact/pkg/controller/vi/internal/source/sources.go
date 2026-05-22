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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

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
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass.")
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

		sc, err := disk.GetStorageClass(ctx, vi.Status.StorageClassName)
		if err != nil {
			return reconcile.Result{}, err
		}
		err = disk.StartSupplementPVCImport(ctx, diskSize, sc, source, vi, supgen, nil)
		if updated, err := setPhaseConditionFromStorageError(err, vi, cb); err != nil || updated {
			return reconcile.Result{}, err
		}

		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("PVC Provisioner not found: create the new one.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	phase, err := disk.EnsureSupplementPVCImport(ctx, pvc, source, vi, supgen, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	vi.Status.Target.PersistentVolumeClaim = pvc.Name
	switch phase {
	case corev1.PodSucceeded:
		vi.Status.Phase = v1alpha2.ImageReady
		cb.Status(metav1.ConditionTrue).Reason(vicondition.Ready).Message("")
		vi.Status.Progress = "100%"
		vi.Status.Size = stat.GetSize(pod)
		vi.Status.CDROM = stat.GetCDROM(pod)
		vi.Status.DownloadSpeed = stat.GetDownloadSpeed(vi.GetUID(), pod)
		return reconcile.Result{RequeueAfter: time.Second}, nil
	case corev1.PodFailed:
		vi.Status.Phase = v1alpha2.ImageFailed
		cb.Status(metav1.ConditionFalse).Reason(vicondition.ProvisioningFailed).Message("VirtualImage importer Pod failed.")
		return reconcile.Result{}, nil
	default:
		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("Import is in the process of provisioning to PVC.")
		vi.Status.Progress = "50.0%"
		if err := disk.Protect(ctx, supgen, vi, pvc); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}
}

func getPVCSizeFromPod(stat Stat, pod *corev1.Pod) (resource.Quantity, error) {
	unpackedSize, err := resource.ParseQuantity(stat.GetSize(pod).UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", stat.GetSize(pod).UnpackedBytes, err)
	}
	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}
	return service.GetValidatedPVCSize(&unpackedSize, unpackedSize)
}

func reconcilePVCImportFromReadySource(
	ctx context.Context,
	vi *v1alpha2.VirtualImage,
	pvc *corev1.PersistentVolumeClaim,
	source *service.PVCImportSource,
	size resource.Quantity,
	cb *conditions.ConditionBuilder,
	supgen supplements.Generator,
	disk *service.DiskService,
	ready func(),
) (reconcile.Result, error) {
	if pvc == nil {
		sc, err := disk.GetStorageClass(ctx, vi.Status.StorageClassName)
		if err != nil {
			return reconcile.Result{}, err
		}
		err = disk.StartSupplementPVCImport(ctx, size, sc, source, vi, supgen, nil)
		if updated, err := setPhaseConditionFromStorageError(err, vi, cb); err != nil || updated {
			return reconcile.Result{}, err
		}
		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("PVC Provisioner not found: create the new one.")
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	phase, err := disk.EnsureSupplementPVCImport(ctx, pvc, source, vi, supgen, nil)
	if err != nil {
		return reconcile.Result{}, err
	}
	vi.Status.Target.PersistentVolumeClaim = pvc.Name
	switch phase {
	case corev1.PodSucceeded:
		vi.Status.Phase = v1alpha2.ImageReady
		cb.Status(metav1.ConditionTrue).Reason(vicondition.Ready).Message("")
		vi.Status.Progress = "100%"
		ready()
		return reconcile.Result{RequeueAfter: time.Second}, nil
	case corev1.PodFailed:
		vi.Status.Phase = v1alpha2.ImageFailed
		cb.Status(metav1.ConditionFalse).Reason(vicondition.ProvisioningFailed).Message("VirtualImage importer Pod failed.")
		return reconcile.Result{}, nil
	default:
		vi.Status.Phase = v1alpha2.ImageProvisioning
		vi.Status.Progress = "0%"
		cb.Status(metav1.ConditionFalse).Reason(vicondition.Provisioning).Message("Import is in the process of provisioning to PVC.")
		if err := disk.Protect(ctx, supgen, vi, pvc); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{RequeueAfter: time.Second}, nil
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
