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
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

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

type SupplementsCleaner interface {
	CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error)
}

func CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk, c SupplementsCleaner) (reconcile.Result, error) {
	if object.ShouldCleanupSubResources(vd) {
		return c.CleanUpSupplements(ctx, vd)
	}

	return reconcile.Result{}, nil
}

func IsDiskProvisioningFinished(c metav1.Condition) bool {
	return c.Reason == vdcondition.Ready.String() || c.Reason == vdcondition.Lost.String()
}

func SetPhaseConditionForFinishedDisk(
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

func setPhaseConditionFromPodError(
	ctx context.Context,
	podErr error,
	pod *corev1.Pod,
	vd *virtv2.VirtualDisk,
	cb *conditions.ConditionBuilder,
	c client.Client,
) error {
	switch {
	case errors.Is(podErr, service.ErrNotInitialized):
		vd.Status.Phase = virtv2.DiskFailed
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ProvisioningNotStarted).
			Message(service.CapitalizeFirstLetter(podErr.Error()) + ".")
		return nil
	case errors.Is(podErr, service.ErrNotScheduled):
		vd.Status.Phase = virtv2.DiskPending

		nodePlacement, err := getNodePlacement(ctx, c, vd)
		if err != nil {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return fmt.Errorf("failed to get importer tolerations: %w", err)
		}

		var isChanged bool
		isChanged, err = provisioner.IsNodePlacementChanged(nodePlacement, pod)
		if err != nil {
			setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return err
		}

		if isChanged {
			err = c.Delete(ctx, pod)
			if err != nil {
				setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", err))
				return err
			}

			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ProvisioningNotStarted).
				Message("Provisioner recreation due to a changes in the virtual machine tolerations.")
		} else {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.ProvisioningNotStarted).
				Message(service.CapitalizeFirstLetter(podErr.Error()) + ".")
		}

		return nil
	case errors.Is(podErr, service.ErrProvisioningFailed):
		setPhaseConditionToFailed(cb, &vd.Status.Phase, podErr)
		return nil
	default:
		setPhaseConditionToFailed(cb, &vd.Status.Phase, fmt.Errorf("unexpected error: %w", podErr))
		return podErr
	}
}

type Cleaner interface {
	CleanUp(ctx context.Context, sup *supplements.Generator) (bool, error)
}

func setPhaseConditionFromProvisioningError(
	ctx context.Context,
	provisioningErr error,
	cb *conditions.ConditionBuilder,
	vd *virtv2.VirtualDisk,
	dv *cdiv1.DataVolume,
	cleaner Cleaner,
	c client.Client,
) error {
	switch {
	case errors.Is(provisioningErr, service.ErrDataVolumeProvisionerUnschedulable):
		nodePlacement, err := getNodePlacement(ctx, c, vd)
		if err != nil {
			err = errors.Join(provisioningErr, err)
			setPhaseConditionToFailed(cb, &vd.Status.Phase, err)
			return err
		}

		isChanged, err := provisioner.IsNodePlacementChanged(nodePlacement, dv)
		if err != nil {
			err = errors.Join(provisioningErr, err)
			setPhaseConditionToFailed(cb, &vd.Status.Phase, err)
			return err
		}

		vd.Status.Phase = virtv2.DiskProvisioning

		if isChanged {
			supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

			_, err = cleaner.CleanUp(ctx, supgen)
			if err != nil {
				err = errors.Join(provisioningErr, err)
				setPhaseConditionToFailed(cb, &vd.Status.Phase, err)
				return err
			}

			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Provisioning).
				Message("PVC provisioner recreation due to a changes in the virtual machine tolerations.")
		} else {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.Provisioning).
				Message("Trying to schedule the PVC provisioner.")
		}

		return nil
	default:
		setPhaseConditionToFailed(cb, &vd.Status.Phase, provisioningErr)
		return provisioningErr
	}
}

func getNodePlacement(ctx context.Context, c client.Client, vd *virtv2.VirtualDisk) (*provisioner.NodePlacement, error) {
	if len(vd.Status.AttachedToVirtualMachines) != 1 {
		return nil, nil
	}

	vmKey := types.NamespacedName{Name: vd.Status.AttachedToVirtualMachines[0].Name, Namespace: vd.Namespace}
	vm, err := object.FetchObject(ctx, vmKey, c, &virtv2.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("unable to get the virtual machine %s: %w", vmKey, err)
	}

	if vm == nil {
		return nil, nil
	}

	var nodePlacement provisioner.NodePlacement
	nodePlacement.Tolerations = append(nodePlacement.Tolerations, vm.Spec.Tolerations...)

	vmClassKey := types.NamespacedName{Name: vm.Spec.VirtualMachineClassName}
	vmClass, err := object.FetchObject(ctx, vmClassKey, c, &virtv2.VirtualMachineClass{})
	if err != nil {
		return nil, fmt.Errorf("unable to get the virtual machine class %s: %w", vmClassKey, err)
	}

	if vmClass == nil {
		return &nodePlacement, nil
	}

	nodePlacement.Tolerations = append(nodePlacement.Tolerations, vmClass.Spec.Tolerations...)

	return &nodePlacement, nil
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

const (
	DVRunningConditionType          cdiv1.DataVolumeConditionType = "Running"
	DVQoutaNotExceededConditionType cdiv1.DataVolumeConditionType = "QuotaNotExceeded"

	DVImagePullFailedReason = "ImagePullFailed"
)
