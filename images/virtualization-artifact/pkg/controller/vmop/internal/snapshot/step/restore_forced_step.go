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
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
)

type Restorer interface {
	RestoreVirtualMachine(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachine, error)
	RestoreProvisioner(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error)
	RestoreVirtualMachineIPAddress(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachineIPAddress, error)
	RestoreVirtualMachineBlockDeviceAttachments(ctx context.Context, secret *corev1.Secret) ([]*virtv2.VirtualMachineBlockDeviceAttachment, error)
}

type ResoreForcedStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	restorer Restorer
	cb       *conditions.ConditionBuilder
	vmop     *virtv2.VirtualMachineOperation
}

func NewResoreForcedStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	vmop *virtv2.VirtualMachineOperation,
) *ResoreForcedStep {
	return &ResoreForcedStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
		vmop:     vmop,
	}
}

func (s ResoreForcedStep) Take(ctx context.Context, vm *virtv2.VirtualMachine) (*reconcile.Result, error) {
	vmRestore := &virtv2.VirtualMachineRestore{}

	switch vmRestore.Status.Phase {
	case virtv2.VirtualMachineRestorePhaseReady,
		virtv2.VirtualMachineRestorePhaseFailed,
		virtv2.VirtualMachineRestorePhaseTerminating:
		return &reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmrestorecondition.VirtualMachineRestoreReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmRestore.Generation), &vmRestore.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmRestore.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	var tocreate []client.object

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
				return &reconcile.Result{}, nil
			case errors.Is(err, restorer.ErrAlreadyExists):
			default:
				setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
				return &reconcile.Result{}, err
			}
		}

		vmObj, err := object.FetchObject(ctx, types.NamespacedName{Name: overridedVMName, Namespace: restoredVM.Namespace}, s.client, &virtv2.VirtualMachine{})
		if err != nil {
			setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
			return &reconcile.Result{}, fmt.Errorf("failed to fetch the `VirtualMachine`: %w", err)
		}

		if vmObj == nil {
			err := errors.New("restoration with `Forced` mode can be applied only to an existing virtual machine; you can restore the virtual machine with `Safe` mode")
			setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
			return &reconcile.Result{}, err
		} else {
			switch vmObj.Status.Phase {
			case virtv2.MachinePending:
				err := errors.New("a virtual machine cannot be restored from the pending phase with `Forced` mode; you can delete the virtual machine and restore it with `Safe` mode")
				setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
				return &reconcile.Result{}, err
			case virtv2.MachineStopped:
			default:
				if runPolicy != virtv2.AlwaysOffPolicy {
					err := updateVMRunPolicy(ctx, s.client, vmObj, virtv2.AlwaysOffPolicy)
					if err != nil {
						if errors.Is(err, restorer.ErrUpdating) {
							setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineIsNotStopped, err.Error())
							return &reconcile.Result{}, nil
						}
						setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
						return &reconcile.Result{}, err
					}
					setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineIsNotStopped, "waiting for the virtual machine run policy will be updated")
					return &reconcile.Result{}, nil
				}

				err := stopVirtualMachine(ctx, s.client, restoredVM.Name, restoredVM.Namespace, string(vmRestore.UID))
				if err != nil {
					if errors.Is(err, restorer.ErrIncomplete) {
						setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineIsNotStopped, "waiting for the virtual machine will be stopped")
						return &reconcile.Result{}, nil
					}
					setPhaseConditionToFailed(cb, &vmRestore.Status.Phase, err)
					return &reconcile.Result{}, err
				}
			}
		}

		for _, ov := range overrideValidators {
			err := ov.ProcessWithForce(ctx)
			switch {
			case err == nil:
			case errors.Is(err, restorer.ErrRestoring), errors.Is(err, restorer.ErrUpdating):
				setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
				return &reconcile.Result{}, nil
			default:
				setPhaseConditionToPending(cb, &vmRestore.Status.Phase, vmrestorecondition.VirtualMachineResourcesAreNotReady, err.Error())
				return &reconcile.Result{}, err
			}
		}
	}

	return &reconcile.Result{}, nil
}
