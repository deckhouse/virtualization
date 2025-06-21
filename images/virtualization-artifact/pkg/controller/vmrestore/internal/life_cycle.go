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

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmrestore/internal/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

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

func (h LifeCycleHandler) Handle(ctx context.Context, vmRestore *virtv2.VirtualMachineRestore) (reconcile.Result, error) {
	switch vmRestore.Status.Phase {
	case virtv2.VirtualMachineRestorePhaseReady,
		virtv2.VirtualMachineRestorePhaseFailed,
		virtv2.VirtualMachineRestorePhaseTerminating:
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmrestorecondition.VirtualMachineRestoreReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmRestore.Generation), &vmRestore.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmRestore.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	if vmRestore.Status.Phase == "" {
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhasePending
	}

	if vmRestore.DeletionTimestamp != nil {
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseTerminating
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
		return reconcile.Result{}, nil
	}

	// ctrlRevRes, err := h.getCtrlRev(ctx, vmRestore)
	// if err != nil {
	// 	return reconcile.Result{}, err
	// }

	if vmRestore.Status.Phase == virtv2.VirtualMachineRestorePhaseInProgress {
		// if vmRestore.Spec.RestoreForcefully {
		// 	err := h.clearVMRestoreVMOPs(ctx, ctrlRevRes.VM.Name, ctrlRevRes.VM.Namespace, virtv2.VMOPTypeStop)
		// 	h.recorder.Event(
		// 		vmRestore,
		// 		corev1.EventTypeWarning,
		// 		virtv2.ReasonVDSpecHasBeenChanged,
		// 		"Failed to remove the `VirtualMachineOperations` after restoring; please remove it manually in the near future.",
		// 	)
		// 	// TODO: replace with warn event
		// 	if err != nil {
		// 		return reconcile.Result{}, err
		// 	}

		err := h.startVirtualMachine(ctx, vmRestore)
		if err != nil {
			h.recorder.Event(
				vmRestore,
				corev1.EventTypeWarning,
				virtv2.ReasonVMStartFailed,
				err.Error(),
			)
		}

		// for _, vd := range ctrlRevRes.VDs {
		// 	err := removeVMRestoreAnnotation(ctx, h.client, &virtv2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: vd.Name, Namespace: vd.Namespace}})
		// 	if err != nil {
		// 		return reconcile.Result{}, err
		// 	}
		// }

		// for _, vmbda := range ctrlRevRes.VMBDAs {
		// 	err := removeVMRestoreAnnotation(ctx, h.client, &virtv2.VirtualMachineBlockDeviceAttachment{ObjectMeta: metav1.ObjectMeta{Name: vmbda.Name, Namespace: vmbda.Namespace}})
		// 	if err != nil {
		// 		return reconcile.Result{}, err
		// 	}
		// }

		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseReady
		cb.Status(metav1.ConditionTrue).Reason(vmrestorecondition.VirtualMachineRestoreReady)

		return reconcile.Result{}, nil
	}

	vmSnapshotReadyToUseCondition, _ := conditions.GetCondition(vmrestorecondition.VirtualMachineSnapshotReadyToUseType, vmRestore.Status.Conditions)
	if vmSnapshotReadyToUseCondition.Status != metav1.ConditionTrue {
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReadyToUse).
			Message(fmt.Sprintf("Waiting for the virtual machine snapshot %q to be ready to use.", vmRestore.Spec.VirtualMachineSnapshotName))
		return reconcile.Result{}, nil
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmRestore.Namespace, Name: vmRestore.Spec.VirtualMachineSnapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, h.client, &virtv2.VirtualMachineSnapshot{})
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

	var overrideValidators []OverrideValidator

	vm, err := h.restorer.RestoreVirtualMachine(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	vmip, err := h.restorer.RestoreVirtualMachineIPAddress(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	if vmip != nil {
		vm.Spec.VirtualMachineIPAddress = vmip.Name
		overrideValidators = append(overrideValidators, restorer.NewVirtualMachineIPAddressOverrideValidator(vmip, h.client))
	}

	overrideValidators = append(overrideValidators, restorer.NewVirtualMachineOverrideValidator(vm, h.client))

	vds, err := h.getVirtualDisks(ctx, vmSnapshot)
	switch {
	case err == nil:
	case errors.Is(err, ErrVirtualDiskSnapshotNotFound):
		vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhasePending
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
		overrideValidators = append(overrideValidators, restorer.NewVirtualDiskOverrideValidator(vd, h.client))
	}

	for _, vmbda := range vmbdas {
		overrideValidators = append(overrideValidators, restorer.NewVirtualMachineBlockDeviceAttachmentsOverrideValidator(vmbda, h.client))
	}

	if provisioner != nil {
		overrideValidators = append(overrideValidators, restorer.NewProvisionerOverrideValidator(provisioner, h.client))
	}

	// if ctrlRevRes == nil {
	// 	ctrlrev, err := h.generateCtrlRev(vmRestore, vm, vds, vmbdas)
	// 	if err != nil {
	// 		return reconcile.Result{}, err
	// 	}
	// 	err = h.client.Create(ctx, ctrlrev)
	// 	if err != nil {
	// 		return reconcile.Result{}, fmt.Errorf("failed to create the `ControllerRevision`: %w", err)
	// 	}
	// }

	var toCreate []client.Object

	if vmRestore.Spec.RestoreMode == virtv2.RestoreModeForced {
		for _, ov := range overrideValidators {
			ov.Override(vmRestore.Spec.NameReplacements)

			err := ov.ValidateWithForce(ctx)
			switch {
			case err == nil:
				toCreate = append(toCreate, ov.Object())
			case errors.Is(err, restorer.ErrAlreadyInUse), errors.Is(err, restorer.ErrAlreadyExistsAndHasDiff):
				vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseFailed
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

		// TODO: add if isVirtualMachineStopped {}
		vmObj, err := object.FetchObject(ctx, types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, h.client, &virtv2.VirtualMachine{})
		if err != nil {
			setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
			return reconcile.Result{}, fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
		}

		if vmObj != nil && vmObj.Status.Phase != virtv2.MachineStopped {
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

		for _, ov := range overrideValidators {
			// This is required to prevent the deletion of resources created in this process.
			ov.AnnotateObject(string(vmRestore.UID))

			err := ov.ProcessWithForce(ctx, string(vmRestore.UID))
			switch {
			case err == nil:
			case errors.Is(err, restorer.ErrRestoring), errors.Is(err, restorer.ErrUpdating):
				setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
				return reconcile.Result{Requeue: true}, nil
			default:
				setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
				return reconcile.Result{Requeue: true}, nil
			}
		}
	}

	if vmRestore.Spec.RestoreMode == virtv2.RestoreModeSafe {
		for _, ov := range overrideValidators {
			ov.Override(vmRestore.Spec.NameReplacements)

			err = ov.Validate(ctx)
			switch {
			case err == nil:
			case errors.Is(err, restorer.ErrAlreadyExists), errors.Is(err, restorer.ErrAlreadyInUse), errors.Is(err, restorer.ErrAlreadyExistsAndHasDiff):
				vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseFailed
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

	err = h.createBatch(ctx, toCreate...)
	if err != nil {
		setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
		return reconcile.Result{}, err
	}

	vmRestore.Status.Phase = virtv2.VirtualMachineRestorePhaseInProgress
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmrestorecondition.VirtualMachineSnapshotNotReady).
		Message(fmt.Sprintf("The virtual machine %q is in the process of restore.", vmSnapshot.Spec.VirtualMachineName))
	return reconcile.Result{Requeue: true}, nil
}

type OverrideValidator interface {
	Object() client.Object
	Override(rules []virtv2.NameReplacement)
	Validate(ctx context.Context) error
	ValidateWithForce(ctx context.Context) error
	ProcessWithForce(ctx context.Context, vmRestoreUID string) error
	AnnotateObject(vmRestoreUID string)
}

var ErrVirtualDiskSnapshotNotFound = errors.New("not found")

func (h LifeCycleHandler) getVirtualDisks(ctx context.Context, vmSnapshot *virtv2.VirtualMachineSnapshot) ([]*virtv2.VirtualDisk, error) {
	vds := make([]*virtv2.VirtualDisk, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))

	for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
		vdSnapshotKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vdSnapshotName}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, h.client, &virtv2.VirtualDiskSnapshot{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch the virtual disk snapshot %q: %w", vdSnapshotKey.Name, err)
		}

		if vdSnapshot == nil {
			return nil, fmt.Errorf("the virtual disk snapshot %q %w", vdSnapshotName, ErrVirtualDiskSnapshotNotFound)
		}

		vd := virtv2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       virtv2.VirtualDiskKind,
				APIVersion: virtv2.Version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdSnapshot.Spec.VirtualDiskName,
				Namespace: vdSnapshot.Namespace,
			},
			Spec: virtv2.VirtualDiskSpec{
				DataSource: &virtv2.VirtualDiskDataSource{
					Type: virtv2.DataSourceTypeObjectRef,
					ObjectRef: &virtv2.VirtualDiskObjectRef{
						Kind: virtv2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshot.Name,
					},
				},
			},
			Status: virtv2.VirtualDiskStatus{
				AttachedToVirtualMachines: []virtv2.AttachedVirtualMachine{
					{Name: vmSnapshot.Spec.VirtualMachineName, Mounted: true},
				},
			},
		}

		vds = append(vds, &vd)
	}

	return vds, nil
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

func setPhaseConditionToFailed(cb *conditions.ConditionBuilder, phase *virtv2.VirtualMachineRestorePhase, err error) {
	*phase = virtv2.VirtualMachineRestorePhaseFailed
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmrestorecondition.VirtualMachineRestoreFailed).
		Message(service.CapitalizeFirstLetter(err.Error()) + ".")
}

func setPhaseConditionToPending(cb *conditions.ConditionBuilder, phase *virtv2.VirtualMachineRestorePhase, reason vmrestorecondition.VirtualMachineRestoreReadyReason, msg string) {
	*phase = virtv2.VirtualMachineRestorePhasePending
	cb.
		Status(metav1.ConditionFalse).
		Reason(reason).
		Message(service.CapitalizeFirstLetter(msg) + ".")
}

func newVMRestoreVMOP(vmName, namespace, vmRestoreUID string, vmopType virtv2.VMOPType) *virtv2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName("vmrestore-"),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMRestore, vmRestoreUID),
		vmopbuilder.WithType(vmopType),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func (h LifeCycleHandler) getVMRestoreVMOP(ctx context.Context, vmNamespace, vmRestoreUID string, vmopType virtv2.VMOPType) (*virtv2.VirtualMachineOperation, error) {
	vmops := &virtv2.VirtualMachineOperationList{}
	err := h.client.List(ctx, vmops, &client.ListOptions{Namespace: vmNamespace})
	if err != nil {
		return nil, err
	}

	for _, vmop := range vmops.Items {
		if v, ok := vmop.Annotations[annotations.AnnVMRestore]; ok {
			if v == vmRestoreUID && vmop.Spec.Type == vmopType {
				return &vmop, nil
			}
		}
	}

	return nil, nil
}

func (h LifeCycleHandler) stopVirtualMachine(ctx context.Context, vmName, vmNamespace, vmRestoreUID string) error {
	vmopStop, err := h.getVMRestoreVMOP(ctx, vmNamespace, vmRestoreUID, virtv2.VMOPTypeStop)
	if err != nil {
		return fmt.Errorf("failed to list the `VirtualMachineOperations`: %w", err)
	}

	if vmopStop == nil {
		vmopStop := newVMRestoreVMOP(vmName, vmNamespace, vmRestoreUID, virtv2.VMOPTypeStop)
		err := h.client.Create(ctx, vmopStop)
		if err != nil {
			return fmt.Errorf("failed to stop the `VirtualMachine`: %w", err)
		}
		return fmt.Errorf("the status of the virtual machine operation is %w", restorer.ErrIncomplete)
	}

	conditionCompleted, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmopStop.Status.Conditions)
	switch vmopStop.Status.Phase {
	case virtv2.VMOPPhaseFailed:
		return fmt.Errorf("failed to stop the `VirtualMachine`: %s", conditionCompleted.Message)
	case virtv2.VMOPPhaseCompleted:
		return fmt.Errorf("failed to stop the `VirtualMachine`; ensure that no other `VirtualMachineOperation` with a completed status exists: %s", vmopStop.Name)
	default:
		return fmt.Errorf("the status of the `VirtualMachineOperation` is %w: %s", restorer.ErrIncomplete, conditionCompleted.Message)
	}
}

func (h LifeCycleHandler) startVirtualMachine(ctx context.Context, vmRestore *virtv2.VirtualMachineRestore) error {
	vms := &virtv2.VirtualMachineList{}
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
	vmObj, err := object.FetchObject(ctx, vmKey, h.client, &virtv2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
	}

	if vmObj != nil {
		if vmObj.Status.Phase == virtv2.MachineStopped {
			vmopStart := newVMRestoreVMOP(vmName, vmRestore.Namespace, string(vmRestore.UID), virtv2.VMOPTypeStart)
			err := h.client.Create(ctx, vmopStart)
			if err != nil {
				return fmt.Errorf("failed to start the `VirtualMachine`: %w", err)
			}
		}
	}

	return nil
}

// func (h LifeCycleHandler) clearVMRestoreVMOPs(ctx context.Context, vmName, vmNamespace string, vmopType virtv2.VMOPType) error {
// 	vmops := &virtv2.VirtualMachineOperationList{}
// 	err := h.client.List(ctx, vmops, &client.ListOptions{Namespace: vmNamespace})
// 	if err != nil {
// 		return fmt.Errorf("failed to list the `VirtualMachineOperations`: %w", err)
// 	}

// 	for _, vmop := range vmops.Items {
// 		if _, ok := vmop.Annotations[annotations.AnnVMOPVMRestore]; !ok {
// 			continue
// 		}

// 		if vmop.Spec.Type == vmopType && vmop.Spec.VirtualMachine == vmName {
// 			err := object.DeleteObject(ctx, h.client, &vmop)
// 			if err != nil {
// 				return fmt.Errorf("failed to delete the `VirtualMachineOperation` %s: %w", vmop.Name, err)
// 			}
// 		}
// 	}

// 	return nil
// }

// type ctrlrevResources struct {
// 	VM     ctrlrevVirtualMachine                        `json:"vm"`
// 	VDs    []ctrlrevVirtualDisk                         `json:"vds"`
// 	VMBDAs []ctrlrevVirtualMachineBlockDeviceAttachment `json:"vmbdas"`
// }

// type ctrlrevVirtualMachine struct {
// 	Name      string `json:"name"`
// 	Namespace string `json:"namespace"`
// }

// type ctrlrevVirtualDisk struct {
// 	Name      string `json:"name"`
// 	Namespace string `json:"namespace"`
// }

// type ctrlrevVirtualMachineBlockDeviceAttachment struct {
// 	Name      string `json:"name"`
// 	Namespace string `json:"namespace"`
// }

// func (h LifeCycleHandler) generateCtrlRev(
// 	vmrestore *virtv2.VirtualMachineRestore,
// 	vm *virtv2.VirtualMachine,
// 	vds []*virtv2.VirtualDisk,
// 	vmbdas []*virtv2.VirtualMachineBlockDeviceAttachment,
// ) (*appsv1.ControllerRevision, error) {
// 	ctrlRevRes := &ctrlrevResources{}

// 	ctrlRevRes.VM = ctrlrevVirtualMachine{
// 		Name:      vm.Name,
// 		Namespace: vm.Namespace,
// 	}

// 	ctrlRevRes.VDs = make([]ctrlrevVirtualDisk, 0, len(vds))
// 	ctrlRevRes.VMBDAs = make([]ctrlrevVirtualMachineBlockDeviceAttachment, 0, len(vmbdas))

// 	for _, vd := range vds {
// 		ctrlRevRes.VDs = append(ctrlRevRes.VDs, ctrlrevVirtualDisk{Name: vd.Name, Namespace: vd.Namespace})
// 	}

// 	for _, vmbda := range vmbdas {
// 		ctrlRevRes.VMBDAs = append(ctrlRevRes.VMBDAs, ctrlrevVirtualMachineBlockDeviceAttachment{Name: vmbda.Name, Namespace: vmbda.Namespace})
// 	}

// 	data, err := json.Marshal(ctrlRevRes)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to marshal a `ControllerRevision` data: %w", err)
// 	}

// 	return &appsv1.ControllerRevision{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      string(vmrestore.UID),
// 			Namespace: vmrestore.Namespace,
// 		},
// 		Data: runtime.RawExtension{Raw: data},
// 	}, nil
// }

// func (h LifeCycleHandler) getCtrlRev(ctx context.Context, vmRestore *virtv2.VirtualMachineRestore) (*ctrlrevResources, error) {
// 	ctrlrevKey := types.NamespacedName{Name: string(vmRestore.UID), Namespace: vmRestore.Namespace}
// 	ctrlrev, err := object.FetchObject(ctx, ctrlrevKey, h.client, &appsv1.ControllerRevision{})
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to fetch the `ControllerRevision`: %w", err)
// 	}

// 	if ctrlrev != nil {
// 		ctrlRevRes := &ctrlrevResources{}
// 		err := json.Unmarshal(ctrlrev.Data.Raw, ctrlRevRes)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to unmarshal a `ControllerRevision` data: %w", err)
// 		}
// 		return ctrlRevRes, nil
// 	}

// 	return nil, nil
// }

// func removeVMRestoreAnnotation[T client.Object](ctx context.Context, c client.Client, obj T) error {
// 	objKey := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
// 	err := c.Get(ctx, objKey, obj)
// 	if err != nil {
// 		return client.IgnoreNotFound(fmt.Errorf("failed to fetch the `%s`: %w", obj.GetObjectKind(), err))
// 	}

// 	err = object.RemoveAnnotation(ctx, c, obj, annotations.AnnVMRestore)
// 	if err != nil {
// 		return fmt.Errorf("failed to remove the `VirtualMachineRestore` annotation: %w", err)
// 	}

// 	return nil
// }
