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

package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmrestore/internal/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const vdPrefix = "vd-"

type LifeCycleHandler struct {
	client   client.Client
	restorer Restorer
	recorder eventrecord.EventRecorderLogger
}

func NewLifeCycleHandler(client client.Client, restorer Restorer, recorder eventrecord.EventRecorderLogger) *LifeCycleHandler {
	return &LifeCycleHandler{
		client:   client,
		restorer: restorer,
		recorder: recorder,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vmRestore *v1alpha2.VirtualMachineRestore) (reconcile.Result, error) {
	switch vmRestore.Status.Phase {
	case v1alpha2.VirtualMachineRestorePhaseReady,
		v1alpha2.VirtualMachineRestorePhaseFailed,
		v1alpha2.VirtualMachineRestorePhaseTerminating:
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmrestorecondition.VirtualMachineRestoreReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmRestore.Generation), &vmRestore.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmRestore.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	if vmRestore.Status.Phase == "" {
		vmRestore.Status.Phase = v1alpha2.VirtualMachineRestorePhasePending
	}

	if vmRestore.DeletionTimestamp != nil {
		vmRestore.Status.Phase = v1alpha2.VirtualMachineRestorePhaseTerminating
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
		return reconcile.Result{}, nil
	}

	if vmRestore.Status.Phase == v1alpha2.VirtualMachineRestorePhaseInProgress {
		if vmRestore.Spec.RestoreMode == v1alpha2.RestoreModeForced {
			err := h.startVirtualMachine(ctx, vmRestore)
			if err != nil {
				h.recorder.Event(
					vmRestore,
					corev1.EventTypeWarning,
					v1alpha2.ReasonVMStartFailed,
					err.Error(),
				)
			}
		}

		vmRestore.Status.Phase = v1alpha2.VirtualMachineRestorePhaseReady
		cb.Status(metav1.ConditionTrue).Reason(vmrestorecondition.VirtualMachineRestoreReady)

		return reconcile.Result{}, nil
	}

	vmSnapshotReadyToUseCondition, _ := conditions.GetCondition(vmrestorecondition.VirtualMachineSnapshotReadyToUseType, vmRestore.Status.Conditions)
	if vmSnapshotReadyToUseCondition.Status != metav1.ConditionTrue {
		vmRestore.Status.Phase = v1alpha2.VirtualMachineRestorePhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReadyToUse).
			Message(fmt.Sprintf("Waiting for the virtual machine snapshot %q to be ready to use.", vmRestore.Spec.VirtualMachineSnapshotName))
		return reconcile.Result{}, nil
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmRestore.Namespace, Name: vmRestore.Spec.VirtualMachineSnapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, h.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if vmSnapshot == nil {
		err = fmt.Errorf("the virtual machine snapshot %q is nil, please report a bug", vmRestore.Spec.VirtualMachineSnapshotName)
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, h.client, &corev1.Secret{})
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	var (
		overrideValidators []OverrideValidator
		runPolicy          v1alpha2.RunPolicy
		overridedVMName    string
	)

	vm, err := h.restorer.RestoreVirtualMachine(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if vmRestore.Spec.RestoreMode == v1alpha2.RestoreModeForced {
		runPolicy = vm.Spec.RunPolicy
		vm.Spec.RunPolicy = v1alpha2.AlwaysOffPolicy
	}

	vmip, err := h.restorer.RestoreVirtualMachineIPAddress(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if vmip != nil {
		vm.Spec.VirtualMachineIPAddress = vmip.Name
		overrideValidators = append(overrideValidators, restorer.NewVirtualMachineIPAddressOverrideValidator(vmip, h.client, string(vmRestore.UID)))
	}

	vmmacs, err := h.restorer.RestoreVirtualMachineMACAddresses(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	macAddressOrder, err := h.restorer.RestoreMACAddressOrder(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if len(vmmacs) > 0 {
		macAddressNamesByAddress := make(map[string]string)
		for _, vmmac := range vmmacs {
			overrideValidators = append(overrideValidators, restorer.NewVirtualMachineMACAddressOverrideValidator(vmmac, h.client, string(vmRestore.UID)))
			macAddressNamesByAddress[vmmac.Status.Address] = vmmac.Name
		}

		for i := range vm.Spec.Networks {
			ns := &vm.Spec.Networks[i]
			if ns.Type == v1alpha2.NetworksTypeMain {
				continue
			}

			ns.VirtualMachineMACAddressName = macAddressNamesByAddress[macAddressOrder[i-1]]
		}
	}

	overrideValidators = append(overrideValidators, restorer.NewVirtualMachineOverrideValidator(vm, h.client, string(vmRestore.UID)))

	overridedVMName, err = h.getOverrridedVMName(overrideValidators)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	vds, err := h.getVirtualDisks(ctx, vmSnapshot)
	switch {
	case err == nil:
	case errors.Is(err, ErrVirtualDiskSnapshotNotFound):
		vmRestore.Status.Phase = v1alpha2.VirtualMachineRestorePhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, nil
	default:
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	vmbdas, err := h.restorer.RestoreVirtualMachineBlockDeviceAttachments(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	provisioner, err := h.restorer.RestoreProvisioner(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	for _, vd := range vds {
		overrideValidators = append(overrideValidators, restorer.NewVirtualDiskOverrideValidator(vd, h.client, string(vmRestore.UID)))
	}

	for _, vmbda := range vmbdas {
		overrideValidators = append(overrideValidators, restorer.NewVirtualMachineBlockDeviceAttachmentsOverrideValidator(vmbda, h.client, string(vmRestore.UID)))
	}

	if provisioner != nil {
		overrideValidators = append(overrideValidators, restorer.NewProvisionerOverrideValidator(provisioner, h.client, string(vmRestore.UID)))
	}

	var toCreate []client.Object

	if vmRestore.Spec.RestoreMode == v1alpha2.RestoreModeForced {
		for _, ov := range overrideValidators {
			ov.Override(vmRestore.Spec.NameReplacements)

			err := ov.ValidateWithForce(ctx)
			switch {
			case err == nil:
				toCreate = append(toCreate, ov.Object())
			case errors.Is(err, restorer.ErrAlreadyInUse), errors.Is(err, restorer.ErrAlreadyExistsAndHasDiff):
				vmRestore.Status.Phase = v1alpha2.VirtualMachineRestorePhaseFailed
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmrestorecondition.VirtualMachineRestoreConflict).
					Message(service.CapitalizeFirstLetter(err.Error()) + ".")
				return reconcile.Result{}, nil
			case errors.Is(err, restorer.ErrAlreadyExists):
			default:
				setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
				return reconcile.Result{}, err
			}
		}

		vmObj, err := object.FetchObject(ctx, types.NamespacedName{Name: overridedVMName, Namespace: vm.Namespace}, h.client, &v1alpha2.VirtualMachine{})
		if err != nil {
			setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
			return reconcile.Result{}, fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
		}

		if vmObj == nil {
			err := errors.New("restoration with `Forced` mode can be applied only to an existing virtual machine; you can restore the virtual machine with `Safe` mode")
			setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
			return reconcile.Result{}, err
		} else {
			switch vmObj.Status.Phase {
			case v1alpha2.MachinePending:
				err := errors.New("a virtual machine cannot be restored from the pending phase with `Forced` mode; you can delete the virtual machine and restore it with `Safe` mode")
				setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
				return reconcile.Result{}, err
			case v1alpha2.MachineStopped:
			default:
				if runPolicy != v1alpha2.AlwaysOffPolicy {
					err := h.updateVMRunPolicy(ctx, vmObj, v1alpha2.AlwaysOffPolicy)
					if err != nil {
						if errors.Is(err, restorer.ErrUpdating) {
							setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineIsNotStopped, err.Error())
							return reconcile.Result{}, nil
						}
						setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
						return reconcile.Result{}, err
					}
					setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineIsNotStopped, "waiting for the virtual machine run policy will be updated")
					return reconcile.Result{}, nil
				}

				err := h.stopVirtualMachine(ctx, vm.Name, vm.Namespace, string(vmRestore.UID))
				if err != nil {
					if errors.Is(err, restorer.ErrIncomplete) {
						setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineIsNotStopped, "waiting for the virtual machine will be stopped")
						return reconcile.Result{}, nil
					}
					setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
					return reconcile.Result{}, err
				}
			}
		}

		for _, ov := range overrideValidators {
			err := ov.ProcessWithForce(ctx)
			switch {
			case err == nil:
			case errors.Is(err, restorer.ErrRestoring), errors.Is(err, restorer.ErrUpdating):
				setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
				return reconcile.Result{}, nil
			default:
				setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
				return reconcile.Result{}, err
			}
		}
	}

	if vmRestore.Spec.RestoreMode == v1alpha2.RestoreModeSafe {
		for _, ov := range overrideValidators {
			ov.Override(vmRestore.Spec.NameReplacements)

			err = ov.Validate(ctx)
			switch {
			case err == nil:
			case errors.Is(err, restorer.ErrAlreadyExists), errors.Is(err, restorer.ErrAlreadyInUse), errors.Is(err, restorer.ErrAlreadyExistsAndHasDiff):
				vmRestore.Status.Phase = v1alpha2.VirtualMachineRestorePhaseFailed
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmrestorecondition.VirtualMachineRestoreConflict).
					Message(service.CapitalizeFirstLetter(err.Error()) + ".")
				return reconcile.Result{}, nil
			default:
				setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
				return reconcile.Result{}, err
			}

			toCreate = append(toCreate, ov.Object())
		}
	}

	currentHotplugs, err := h.getCurrentVirtualMachineBlockDeviceAttachments(ctx, vm.Name, vm.Namespace, string(vmRestore.UID))
	if err != nil {
		setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
		return reconcile.Result{}, err
	}

	err = h.deleteCurrentVirtualMachineBlockDeviceAttachments(ctx, currentHotplugs)
	if err != nil {
		setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
		return reconcile.Result{}, err
	}

	err = h.createBatch(ctx, toCreate...)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if vmRestore.Spec.RestoreMode == v1alpha2.RestoreModeForced {
		err = h.checkKVVMDiskStatus(ctx, vm.Name, vm.Namespace)
		if err != nil {
			if errors.Is(err, restorer.ErrRestoring) {
				setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}

		vmObj, err := object.FetchObject(ctx, types.NamespacedName{Name: overridedVMName, Namespace: vm.Namespace}, h.client, &v1alpha2.VirtualMachine{})
		if err != nil {
			setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
			return reconcile.Result{}, fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
		}

		err = h.updateVMRunPolicy(ctx, vmObj, runPolicy)
		if err != nil {
			if errors.Is(err, restorer.ErrUpdating) {
				setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
				return reconcile.Result{}, nil
			}
			setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
			return reconcile.Result{}, err
		}
	}

	vmRestore.Status.Phase = v1alpha2.VirtualMachineRestorePhaseInProgress
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmrestorecondition.VirtualMachineSnapshotNotReady).
		Message(fmt.Sprintf("The virtual machine %q is in the process of restore.", vmSnapshot.Spec.VirtualMachineName))
	return reconcile.Result{}, nil
}

type OverrideValidator interface {
	Object() client.Object
	Override(rules []v1alpha2.NameReplacement)
	Validate(ctx context.Context) error
	ValidateWithForce(ctx context.Context) error
	ProcessWithForce(ctx context.Context) error
}

var ErrVirtualDiskSnapshotNotFound = errors.New("not found")

func (h LifeCycleHandler) getVirtualDisks(ctx context.Context, vmSnapshot *v1alpha2.VirtualMachineSnapshot) ([]*v1alpha2.VirtualDisk, error) {
	vds := make([]*v1alpha2.VirtualDisk, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))

	for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
		vdSnapshotKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vdSnapshotName}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, h.client, &v1alpha2.VirtualDiskSnapshot{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch the virtual disk snapshot %q: %w", vdSnapshotKey.Name, err)
		}

		if vdSnapshot == nil {
			return nil, fmt.Errorf("the virtual disk snapshot %q %w", vdSnapshotName, ErrVirtualDiskSnapshotNotFound)
		}

		vd := v1alpha2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       v1alpha2.VirtualDiskKind,
				APIVersion: v1alpha2.Version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdSnapshot.Spec.VirtualDiskName,
				Namespace: vdSnapshot.Namespace,
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshot.Name,
					},
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
					{Name: vmSnapshot.Spec.VirtualMachineName, Mounted: true},
				},
			},
		}

		vds = append(vds, &vd)
	}

	return vds, nil
}

func (h LifeCycleHandler) getCurrentVirtualMachineBlockDeviceAttachments(ctx context.Context, vmName, vmNamespace, vmRestoreUID string) ([]*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	vmbdas := &v1alpha2.VirtualMachineBlockDeviceAttachmentList{}
	err := h.client.List(ctx, vmbdas, &client.ListOptions{Namespace: vmNamespace})
	if err != nil {
		return nil, fmt.Errorf("failed to list the `VirtualMachineBlockDeviceAttachment`: %w", err)
	}

	vmbdasByVM := make([]*v1alpha2.VirtualMachineBlockDeviceAttachment, 0, len(vmbdas.Items))
	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.VirtualMachineName != vmName {
			continue
		}
		if value, ok := vmbda.Annotations[annotations.AnnVMRestore]; ok && value == vmRestoreUID {
			continue
		}
		vmbdasByVM = append(vmbdasByVM, &vmbda)
	}

	return vmbdasByVM, nil
}

func (h LifeCycleHandler) deleteCurrentVirtualMachineBlockDeviceAttachments(ctx context.Context, vmbdas []*v1alpha2.VirtualMachineBlockDeviceAttachment) error {
	for _, vmbda := range vmbdas {
		err := object.DeleteObject(ctx, h.client, client.Object(vmbda))
		if err != nil {
			return fmt.Errorf("failed to delete the `VirtualMachineBlockDeviceAttachment` %s: %w", vmbda.Name, err)
		}
	}

	return nil
}

func (h LifeCycleHandler) createBatch(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		err := h.client.Create(ctx, obj)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s %q: %w", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), err)
		}
	}

	return nil
}

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *v1alpha2.VirtualMachineRestorePhase, err error) {
	*phase = v1alpha2.VirtualMachineRestorePhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmrestorecondition.VirtualMachineRestoreFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}

func setPhaseConditionToPending(cb *conditions.ConditionBuilder, phase *v1alpha2.VirtualMachineRestorePhase, reason vmrestorecondition.VirtualMachineRestoreReadyReason, msg string) {
	*phase = v1alpha2.VirtualMachineRestorePhasePending
	cb.
		Status(metav1.ConditionFalse).
		Reason(reason).
		Message(service.CapitalizeFirstLetter(msg) + ".")
}

func newVMRestoreVMOP(vmName, namespace, vmRestoreUID string, vmopType v1alpha2.VMOPType) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName("vmrestore-"),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMOPRestore, vmRestoreUID),
		vmopbuilder.WithType(vmopType),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func (h LifeCycleHandler) getVMRestoreVMOP(ctx context.Context, vmNamespace, vmRestoreUID string, vmopType v1alpha2.VMOPType) (*v1alpha2.VirtualMachineOperation, error) {
	vmops := &v1alpha2.VirtualMachineOperationList{}
	err := h.client.List(ctx, vmops, &client.ListOptions{Namespace: vmNamespace})
	if err != nil {
		return nil, err
	}

	for _, vmop := range vmops.Items {
		if v, ok := vmop.Annotations[annotations.AnnVMOPRestore]; ok {
			if v == vmRestoreUID && vmop.Spec.Type == vmopType {
				return &vmop, nil
			}
		}
	}

	return nil, nil
}

func (h LifeCycleHandler) stopVirtualMachine(ctx context.Context, vmName, vmNamespace, vmRestoreUID string) error {
	vmopStop, err := h.getVMRestoreVMOP(ctx, vmNamespace, vmRestoreUID, v1alpha2.VMOPTypeStop)
	if err != nil {
		return fmt.Errorf("failed to list the `VirtualMachineOperations`: %w", err)
	}

	if vmopStop == nil {
		vmopStop := newVMRestoreVMOP(vmName, vmNamespace, vmRestoreUID, v1alpha2.VMOPTypeStop)
		err := h.client.Create(ctx, vmopStop)
		if err != nil {
			return fmt.Errorf("failed to stop the `VirtualMachine`: %w", err)
		}
		return fmt.Errorf("the status of the virtual machine operation is %w", restorer.ErrIncomplete)
	}

	conditionCompleted, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmopStop.Status.Conditions)
	switch vmopStop.Status.Phase {
	case v1alpha2.VMOPPhaseFailed:
		return fmt.Errorf("failed to stop the `VirtualMachine`: %s", conditionCompleted.Message)
	case v1alpha2.VMOPPhaseCompleted:
		return nil
	default:
		return fmt.Errorf("the status of the `VirtualMachineOperation` is %w: %s", restorer.ErrIncomplete, conditionCompleted.Message)
	}
}

func (h LifeCycleHandler) startVirtualMachine(ctx context.Context, vmRestore *v1alpha2.VirtualMachineRestore) error {
	vms := &v1alpha2.VirtualMachineList{}
	err := h.client.List(ctx, vms, &client.ListOptions{Namespace: vmRestore.Namespace})
	if err != nil {
		return fmt.Errorf("failed to list the `VirtualMachines`: %w", err)
	}

	var vmName string
	for _, vm := range vms.Items {
		if v, ok := vm.Annotations[annotations.AnnVMRestore]; ok && v == string(vmRestore.UID) {
			vmName = vm.Name
		}
	}

	vmKey := types.NamespacedName{Name: vmName, Namespace: vmRestore.Namespace}
	vmObj, err := object.FetchObject(ctx, vmKey, h.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	if vmObj != nil {
		if vmObj.Spec.RunPolicy != v1alpha2.AlwaysOnUnlessStoppedManually {
			return nil
		}

		if vmObj.Status.Phase == v1alpha2.MachineStopped {
			vmopStart := newVMRestoreVMOP(vmName, vmRestore.Namespace, string(vmRestore.UID), v1alpha2.VMOPTypeStart)
			err := h.client.Create(ctx, vmopStart)
			if err != nil {
				return fmt.Errorf("failed to start the `VirtualMachine`: %w", err)
			}
		}
	}

	return nil
}

func (h LifeCycleHandler) checkKVVMDiskStatus(ctx context.Context, vmName, vmNamespace string) error {
	kvvmKey := types.NamespacedName{Name: vmName, Namespace: vmNamespace}
	kvvm, err := object.FetchObject(ctx, kvvmKey, h.client, &virtv1.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `InternalVirtualMachine`: %w", err)
	}

	if kvvm != nil {
		for _, vss := range kvvm.Status.VolumeSnapshotStatuses {
			if strings.HasPrefix(vss.Name, vdPrefix) && vss.Reason == restorer.ReasonPVCNotFound {
				return fmt.Errorf("waiting for the `VirtualDisks` %w", restorer.ErrRestoring)
			}
		}
		return nil
	}

	return fmt.Errorf("failed to fetch the `InternalVirtualMachine`: %s", vmName)
}

func (h LifeCycleHandler) getOverrridedVMName(overrideValidators []OverrideValidator) (string, error) {
	for _, ov := range overrideValidators {
		if ov.Object().GetObjectKind().GroupVersionKind().Kind == v1alpha2.VirtualMachineKind {
			return ov.Object().GetName(), nil
		}
	}

	return "", fmt.Errorf("failed to get the `VirtualMachine` name")
}

func (h LifeCycleHandler) updateVMRunPolicy(ctx context.Context, vmObj *v1alpha2.VirtualMachine, runPolicy v1alpha2.RunPolicy) error {
	vmObj.Spec.RunPolicy = runPolicy

	err := h.client.Update(ctx, vmObj)
	if err != nil {
		if k8serrors.IsConflict(err) {
			return fmt.Errorf("waiting for the virtual machine run policy %w", restorer.ErrUpdating)
		} else {
			return fmt.Errorf("failed to update the virtual machine run policy: %w", err)
		}
	}

	return nil
}
