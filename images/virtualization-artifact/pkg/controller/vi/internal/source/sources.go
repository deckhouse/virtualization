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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type Handler interface {
	StoreToDVCR(ctx context.Context, vi *virtv2.VirtualImage) (bool, error)
	StoreToPVC(ctx context.Context, vi *virtv2.VirtualImage) (bool, error)
	CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error)
	Validate(ctx context.Context, vi *virtv2.VirtualImage) error
}

type Sources struct {
	sources map[virtv2.DataSourceType]Handler
}

func NewSources() *Sources {
	return &Sources{
		sources: make(map[virtv2.DataSourceType]Handler),
	}
}

func (s Sources) Set(dsType virtv2.DataSourceType, h Handler) {
	s.sources[dsType] = h
}

func (s Sources) For(dsType virtv2.DataSourceType) (Handler, bool) {
	source, ok := s.sources[dsType]
	return source, ok
}

func (s Sources) Changed(_ context.Context, vi *virtv2.VirtualImage) bool {
	return vi.Generation != vi.Status.ObservedGeneration
}

func (s Sources) CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
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
	CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error)
	CleanUpSupplements(ctx context.Context, vi *virtv2.VirtualImage) (bool, error)
}

func CleanUp(ctx context.Context, vi *virtv2.VirtualImage, c Cleaner) (bool, error) {
	if cc.ShouldCleanupSubResources(vi) {
		return c.CleanUp(ctx, vi)
	}

	return false, nil
}

func CleanUpSupplements(ctx context.Context, vi *virtv2.VirtualImage, c Cleaner) (bool, error) {
	if cc.ShouldCleanupSubResources(vi) {
		return c.CleanUpSupplements(ctx, vi)
	}

	return false, nil
}

func isDiskProvisioningFinished(c metav1.Condition) bool {
	return c.Reason == vicondition.Ready
}

func setPhaseConditionForImporterStart(ready *metav1.Condition, phase *virtv2.ImagePhase, err error) (bool, error) {
	return setPhaseConditionForPodStart(ready, phase, err, virtv2.ImageProvisioning, vicondition.Provisioning)
}

func setPhaseConditionForUploaderStart(ready *metav1.Condition, phase *virtv2.ImagePhase, err error) (bool, error) {
	return setPhaseConditionForPodStart(ready, phase, err, virtv2.ImagePending, vicondition.WaitForUserUpload)
}

func setPhaseConditionForPodStart(ready *metav1.Condition, phase *virtv2.ImagePhase, err error, okPhase virtv2.ImagePhase, okReason vicondition.ReadyReason) (bool, error) {
	switch {
	case err == nil:
		*phase = okPhase
		ready.Status = metav1.ConditionFalse
		ready.Reason = okReason
		ready.Message = "DVCR Provisioner not found: create the new one."
		return true, nil
	case cc.ErrQuotaExceeded(err):
		*phase = virtv2.ImageFailed
		ready.Status = metav1.ConditionFalse
		ready.Reason = cvicondition.ProvisioningFailed
		ready.Message = fmt.Sprintf("Quota exceeded: please configure the `importerResourceRequirements` field in the virtualization module configuration; %s.", err)
		return false, nil
	default:
		*phase = virtv2.ImageFailed
		ready.Status = metav1.ConditionFalse
		ready.Reason = cvicondition.ProvisioningFailed
		ready.Message = fmt.Sprintf("Unexpected error: %s.", err)
		return false, err
	}
}

type CheckImportProcess interface {
	CheckImportProcess(ctx context.Context, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error
}

func setPhaseConditionForFinishedImage(
	pvc *corev1.PersistentVolumeClaim,
	condition *metav1.Condition,
	phase *virtv2.ImagePhase,
	supgen *supplements.Generator,
) {
	switch {
	case pvc == nil:
		*phase = virtv2.ImageLost
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Lost
		condition.Message = fmt.Sprintf("PVC %s not found.", supgen.PersistentVolumeClaim().String())
	default:
		*phase = virtv2.ImageReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""
	}
}

func setPhaseConditionToFailed(ready *metav1.Condition, phase *virtv2.ImagePhase, err error) {
	*phase = virtv2.ImageFailed
	ready.Status = metav1.ConditionFalse
	ready.Reason = vicondition.ProvisioningFailed
	ready.Message = service.CapitalizeFirstLetter(err.Error())
}

func setPhaseConditionForPVCProvisioningImage(
	ctx context.Context,
	dv *cdiv1.DataVolume,
	vi *virtv2.VirtualImage,
	pvc *corev1.PersistentVolumeClaim,
	condition *metav1.Condition,
	checker CheckImportProcess,
) error {
	err := checker.CheckImportProcess(ctx, dv, pvc)
	switch {
	case err == nil:
		if dv == nil {
			vi.Status.Phase = virtv2.ImageProvisioning
			condition.Status = metav1.ConditionFalse
			condition.Reason = vicondition.Provisioning
			condition.Message = "Waiting for the pvc importer to be created"
			return nil
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to PVC."
		return nil
	case errors.Is(err, service.ErrDataVolumeNotRunning):
		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.ProvisioningFailed
		condition.Message = service.CapitalizeFirstLetter(err.Error())
		return nil
	case errors.Is(err, service.ErrStorageClassNotFound):
		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.ProvisioningFailed
		condition.Message = "Provided StorageClass not found in the cluster."
		return nil
	case errors.Is(err, service.ErrDefaultStorageClassNotFound):
		vi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.ProvisioningFailed
		condition.Message = "Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass."
		return nil
	default:
		return err
	}
}

func setPhaseConditionFromPodError(ready *metav1.Condition, vi *virtv2.VirtualImage, err error) error {
	vi.Status.Phase = virtv2.ImageFailed

	switch {
	case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
		ready.Status = metav1.ConditionFalse
		ready.Reason = vicondition.ProvisioningNotStarted
		ready.Message = service.CapitalizeFirstLetter(err.Error() + ".")
		return nil
	case errors.Is(err, service.ErrProvisioningFailed):
		ready.Status = metav1.ConditionFalse
		ready.Reason = vicondition.ProvisioningFailed
		ready.Message = service.CapitalizeFirstLetter(err.Error() + ".")
		return nil
	default:
		return err
	}
}
