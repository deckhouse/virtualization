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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
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

type CheckImportProcess interface {
	CheckImportProcess(ctx context.Context, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error
}

func setPhaseConditionForFinishedImage(
	pvc *corev1.PersistentVolumeClaim,
	cb *conditions.ConditionBuilder,
	phase *v1alpha2.ImagePhase,
	supgen supplements.Generator,
) {
	switch {
	case pvc == nil:
		*phase = v1alpha2.ImageLost
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Lost).
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

func setPhaseConditionForPVCProvisioningImage(
	ctx context.Context,
	dv *cdiv1.DataVolume,
	vi *v1alpha2.VirtualImage,
	pvc *corev1.PersistentVolumeClaim,
	cb *conditions.ConditionBuilder,
	checker CheckImportProcess,
) error {
	err := checker.CheckImportProcess(ctx, dv, pvc)
	switch {
	case err == nil:
		if dv == nil {
			vi.Status.Phase = v1alpha2.ImageProvisioning
			cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.Provisioning).
				Message("Waiting for the pvc importer to be created")
			return nil
		}

		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.Provisioning).
			Message("Import is in the process of provisioning to PVC.")
		return nil
	case errors.Is(err, service.ErrDataVolumeNotRunning):
		vi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return nil
	case errors.Is(err, service.ErrDefaultStorageClassNotFound):
		vi.Status.Phase = v1alpha2.ImagePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.ProvisioningFailed).
			Message("Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass.")
		return nil
	default:
		return err
	}
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

const (
	DVRunningConditionType          cdiv1.DataVolumeConditionType = "Running"
	DVQoutaNotExceededConditionType cdiv1.DataVolumeConditionType = "QuotaNotExceeded"

	DVImagePullFailedReason = "ImagePullFailed"
)
