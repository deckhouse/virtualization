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
	storev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

//go:generate moq -rm -out mock.go . Handler

type Handler interface {
	Name() string
	Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error)
	CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error)
	Validate(ctx context.Context, vd *virtv2.VirtualDisk) error
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

func (s Sources) Get(dsType virtv2.DataSourceType) (Handler, bool) {
	source, ok := s.sources[dsType]
	return source, ok
}

func (s Sources) Changed(_ context.Context, vd *virtv2.VirtualDisk) bool {
	if vd.Generation == 1 {
		return false
	}

	return vd.Generation != vd.Status.ObservedGeneration
}

func (s Sources) CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	var requeue bool

	for _, source := range s.sources {
		ok, err := source.CleanUp(ctx, vd)
		if err != nil {
			return false, fmt.Errorf("clean up failed for data source %s: %w", source.Name(), err)
		}

		requeue = requeue || ok
	}

	return requeue, nil
}

type Cleaner interface {
	CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error)
}

func CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk, c Cleaner) (reconcile.Result, error) {
	if cc.ShouldCleanupSubResources(vd) {
		return c.CleanUpSupplements(ctx, vd)
	}

	return reconcile.Result{}, nil
}

func isDiskProvisioningFinished(c metav1.Condition) bool {
	return c.Reason == vdcondition.Ready.String() || c.Reason == vdcondition.Lost.String()
}

func setPhaseConditionForFinishedDisk(
	pvc *corev1.PersistentVolumeClaim,
	cb *conditions.ConditionBuilder,
	phase *virtv2.DiskPhase,
	supgen *supplements.Generator,
) {
	switch {
	case pvc == nil:
		*phase = virtv2.DiskLost
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Lost).
			Message(fmt.Sprintf("PVC %s not found.", supgen.PersistentVolumeClaim().String()))
	case pvc.Status.Phase == corev1.ClaimLost:
		*phase = virtv2.DiskLost
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Lost).
			Message(fmt.Sprintf("PV %s not found.", pvc.Spec.VolumeName))
	default:
		*phase = virtv2.DiskReady
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Ready).
			Message("")
	}
}

type CheckImportProcess interface {
	CheckImportProcess(ctx context.Context, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error
}

func setConditionFromStorageClassError(err error, cb *conditions.ConditionBuilder) (bool, error) {
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, service.ErrStorageClassNotFound):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("Provided StorageClass not found in the cluster.")
		return true, nil
	case errors.Is(err, service.ErrStorageClassNotAllowed):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("Specified StorageClass is not allowed: please check the module settings.")
		return true, nil
	default:
		return false, err
	}
}

func setPhaseConditionFromStorageError(err error, vd *virtv2.VirtualDisk, cb *conditions.ConditionBuilder) (bool, error) {
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, service.ErrStorageProfileNotFound):
		vd.Status.Phase = virtv2.DiskPending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("StorageProfile not found in the cluster: Please check a StorageClass name in the cluster or set a default StorageClass.")
		return true, nil
	case errors.Is(err, service.ErrStorageClassNotFound):
		vd.Status.Phase = virtv2.DiskPending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("Provided StorageClass not found in the cluster.")
		return true, nil
	case errors.Is(err, service.ErrDefaultStorageClassNotFound):
		vd.Status.Phase = virtv2.DiskPending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message("Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass.")
		return true, nil
	default:
		return false, err
	}
}

func setPhaseConditionForPVCProvisioningDisk(
	ctx context.Context,
	dv *cdiv1.DataVolume,
	vd *virtv2.VirtualDisk,
	pvc *corev1.PersistentVolumeClaim,
	sc *storev1.StorageClass,
	cb *conditions.ConditionBuilder,
	checker CheckImportProcess,
) error {
	err := checker.CheckImportProcess(ctx, dv, pvc)
	switch {
	case err == nil:
		if dv == nil {
			vd.Status.Phase = virtv2.DiskProvisioning
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Provisioning).
				Message("Waiting for the pvc importer to be created")
			return nil
		}
		isWFFC := sc != nil && sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storev1.VolumeBindingWaitForFirstConsumer
		if isWFFC && (dv.Status.Phase == cdiv1.PendingPopulation || dv.Status.Phase == cdiv1.WaitForFirstConsumer) {
			vd.Status.Phase = virtv2.DiskWaitForFirstConsumer
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.WaitingForFirstConsumer).
				Message("The provisioning has been suspended: a created and scheduled virtual machine is awaited")
			return nil
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.Provisioning).
			Message("Import is in the process of provisioning to PVC.")
		return nil
	case errors.Is(err, service.ErrDataVolumeNotRunning):
		vd.Status.Phase = virtv2.DiskFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningFailed).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return nil
	default:
		return err
	}
}

const retryPeriod = 1

func setQuotaExceededPhaseCondition(cb *conditions.ConditionBuilder, phase *virtv2.DiskPhase, err error, creationTimestamp metav1.Time) reconcile.Result {
	*phase = virtv2.DiskFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.ProvisioningFailed)

	if creationTimestamp.Add(30 * time.Minute).After(time.Now()) {
		cb.Message(fmt.Sprintf("Quota exceeded: %s; Please configure quotas or try recreating the resource later.", err))
		return reconcile.Result{}
	}

	cb.Message(fmt.Sprintf("Quota exceeded: %s; Retry in %d minute.", err, retryPeriod))
	return reconcile.Result{RequeueAfter: retryPeriod * time.Minute}
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *virtv2.DiskPhase, err error) {
	*phase = virtv2.DiskFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.ProvisioningFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}
