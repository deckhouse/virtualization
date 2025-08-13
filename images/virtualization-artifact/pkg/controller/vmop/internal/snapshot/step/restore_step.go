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

package step

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/snapshot/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type RestoreVMStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	restorer Restorer
	cb       *conditions.ConditionBuilder
	vmop     *virtv2.VirtualMachineOperation
}

func NewRestoreVMStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	vmop *virtv2.VirtualMachineOperation,
) *RestoreVMStep {
	return &RestoreVMStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
		vmop:     vmop,
	}
}

func (s RestoreVMStep) Take(ctx context.Context, vm *virtv2.VirtualMachine) (*reconcile.Result, error) {
	// TODO:
	vmSnapshot := &virtv2.VirtualMachineSnapshot{}

	switch s.vmop.Status.Phase {
	case virtv2.VMOPPhaseCompleted,
		virtv2.VMOPPhaseFailed,
		virtv2.VMOPPhaseTerminating:
		return &reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmopcondition.TypeCompleted)
	defer func() { conditions.SetCondition(cb.Generation(s.vmop.Generation), &s.vmop.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), s.vmop.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	if s.vmop.Status.Phase == "" {
		s.vmop.Status.Phase = virtv2.VMOPPhasePending
	}

	if s.vmop.DeletionTimestamp != nil {
		s.vmop.Status.Phase = virtv2.VMOPPhaseTerminating
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
		return &reconcile.Result{}, nil
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, s.client, &corev1.Secret{})
	if err != nil {
		setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	var (
		overrideValidators []OverrideValidator
		overridedVMName    string
	)

	restoredVM, err := s.restorer.RestoreVirtualMachine(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	vmip, err := s.restorer.RestoreVirtualMachineIPAddress(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	if vmip != nil {
		restoredVM.Spec.VirtualMachineIPAddress = vmip.Name
		overrideValidators = append(overrideValidators, restorer.NewVirtualMachineIPAddressOverrideValidator(vmip, s.client, string(s.vmop.UID)))
	}

	overrideValidators = append(overrideValidators, restorer.NewVirtualMachineOverrideValidator(restoredVM, s.client, string(s.vmop.UID)))

	overridedVMName, err = getOverrridedVMName(overrideValidators)
	if err != nil {
		setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	vds, err := getVirtualDisks(ctx, s.client, vmSnapshot)
	switch {
	case err == nil:
	case errors.Is(err, ErrVirtualDiskSnapshotNotFound):
		s.vmop.Status.Phase = virtv2.VMOPPhasePending
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmopcondition.ReasonTODO).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return &reconcile.Result{}, nil
	default:
		setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	vmbdas, err := s.restorer.RestoreVirtualMachineBlockDeviceAttachments(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	provisioner, err := s.restorer.RestoreProvisioner(ctx, restorerSecret)
	if err != nil {
		setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	for _, vd := range vds {
		overrideValidators = append(overrideValidators, restorer.NewVirtualDiskOverrideValidator(vd, s.client, string(s.vmop.UID)))
	}

	for _, vmbda := range vmbdas {
		overrideValidators = append(overrideValidators, restorer.NewVirtualMachineBlockDeviceAttachmentsOverrideValidator(vmbda, s.client, string(s.vmop.UID)))
	}

	if provisioner != nil {
		overrideValidators = append(overrideValidators, restorer.NewProvisionerOverrideValidator(provisioner, s.client, string(s.vmop.UID)))
	}

	var toCreate []client.Object

	if s.vmop.Spec.Restore.Mode == virtv2.VMOPRestoreModeBestEffort {
		for _, ov := range overrideValidators {
			// ov.Override(s.vmop.Spec.NameReplacements)

			err := ov.ValidateWithForce(ctx)
			switch {
			case err == nil:
				toCreate = append(toCreate, ov.Object())
			case errors.Is(err, restorer.ErrAlreadyInUse), errors.Is(err, restorer.ErrAlreadyExistsAndHasDiff):
				s.vmop.Status.Phase = virtv2.VMOPPhaseFailed
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmopcondition.ReasonTODO).
					Message(service.CapitalizeFirstLetter(err.Error()) + ".")
				return &reconcile.Result{}, nil
			case errors.Is(err, restorer.ErrAlreadyExists):
			default:
				setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
				return &reconcile.Result{}, err
			}
		}

		vmObj, err := object.FetchObject(ctx, types.NamespacedName{Name: overridedVMName, Namespace: restoredVM.Namespace}, s.client, &virtv2.VirtualMachine{})
		if err != nil {
			setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
			return &reconcile.Result{}, fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
		}

		if vmObj == nil {
			err := errors.New("restoration with `Forced` mode can be applied only to an existing virtual machine; you can restore the virtual machine with `Safe` mode")
			setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
			return &reconcile.Result{}, err
		} else {
			switch vmObj.Status.Phase {
			case virtv2.MachinePending:
				err := errors.New("a virtual machine cannot be restored from the pending phase with `Forced` mode; you can delete the virtual machine and restore it with `Safe` mode")
				setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
				return &reconcile.Result{}, err
			case virtv2.MachineStopped:
			default:
				err := stopVirtualMachine(ctx, s.client, restoredVM.Name, restoredVM.Namespace, string(s.vmop.UID))
				if err != nil {
					if errors.Is(err, restorer.ErrIncomplete) {
						setPhaseConditionToPending(cb, &s.vmop.Status.Phase, vmopcondition.ReasonStopInProgress, "waiting for the virtual machine will be stopped")
						return &reconcile.Result{}, nil
					}
					setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
					return &reconcile.Result{}, err
				}
			}
		}

		for _, ov := range overrideValidators {
			err := ov.ProcessWithForce(ctx)
			switch {
			case err == nil:
			case errors.Is(err, restorer.ErrRestoring), errors.Is(err, restorer.ErrUpdating):
				setPhaseConditionToPending(cb, &s.vmop.Status.Phase, vmopcondition.ReasonTODO, err.Error())
				return &reconcile.Result{}, nil
			default:
				setPhaseConditionToPending(cb, &s.vmop.Status.Phase, vmopcondition.ReasonTODO, err.Error())
				return &reconcile.Result{}, err
			}
		}
	}

	if s.vmop.Spec.Restore.Mode == virtv2.VMOPRestoreModeStrict {
		for _, ov := range overrideValidators {
			// ov.Override(s.vmop.Spec.NameReplacements)

			err = ov.Validate(ctx)
			switch {
			case err == nil:
			case errors.Is(err, restorer.ErrAlreadyExists), errors.Is(err, restorer.ErrAlreadyInUse), errors.Is(err, restorer.ErrAlreadyExistsAndHasDiff):
				s.vmop.Status.Phase = virtv2.VMOPPhaseFailed
				cb.
					Status(metav1.ConditionFalse).
					Reason(vmopcondition.ReasonTODO).
					Message(service.CapitalizeFirstLetter(err.Error()) + ".")
				return &reconcile.Result{}, nil
			default:
				setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
				return &reconcile.Result{}, err
			}

			toCreate = append(toCreate, ov.Object())
		}
	}

	currentHotplugs, err := getCurrentVirtualMachineBlockDeviceAttachments(ctx, s.client, restoredVM.Name, restoredVM.Namespace, string(s.vmop.UID))
	if err != nil {
		setPhaseConditionToPending(cb, &s.vmop.Status.Phase, vmopcondition.ReasonTODO, err.Error())
		return &reconcile.Result{}, err
	}

	err = deleteCurrentVirtualMachineBlockDeviceAttachments(ctx, s.client, currentHotplugs)
	if err != nil {
		setPhaseConditionToPending(cb, &s.vmop.Status.Phase, vmopcondition.ReasonTODO, err.Error())
		return &reconcile.Result{}, err
	}

	err = createBatch(ctx, s.client, toCreate...)
	if err != nil {
		setPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	err = checkKVVMDiskStatus(ctx, s.client, restoredVM.Name, restoredVM.Namespace)
	if err != nil {
		if errors.Is(err, restorer.ErrRestoring) {
			setPhaseConditionToPending(cb, &s.vmop.Status.Phase, vmopcondition.ReasonTODO, err.Error())
			return &reconcile.Result{}, nil
		}
		return &reconcile.Result{}, err
	}

	s.vmop.Status.Phase = virtv2.VMOPPhaseInProgress
	cb.
		Status(metav1.ConditionFalse).
		Reason(vmopcondition.ReasonTODO).
		Message(fmt.Sprintf("The virtual machine %q is in the process of restore.", vmSnapshot.Spec.VirtualMachineName))

	return &reconcile.Result{}, nil
}
