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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StopVMStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
	vmop     *virtv2.VirtualMachineOperation
}

func NewStopVMStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	vmop *virtv2.VirtualMachineOperation,
) *StopVMStep {
	return &StopVMStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
		vmop:     vmop,
	}
}

func (s StopVMStep) Take(ctx context.Context, vm *virtv2.VirtualMachine) (*reconcile.Result, error) {
	// switch vm.Status.Phase {
	// case virtv2.MachinePending:
	// 	// TODO: wat?
	// 	err := errors.New("a virtual machine cannot be restored from the pending phase with `Forced` mode; you can delete the virtual machine and restore it with `Strict` mode")
	// 	setPhaseConditionToFailed(s.cb, &s.vmop.Status.Phase, err)
	// 	return &reconcile.Result{}, err
	// case virtv2.MachineStopped:
	// default:
	// 	err := stopVirtualMachine(ctx, s.client, vm.Name, vm.Namespace, "TODO")
	// 	if err != nil {
	// 		if errors.Is(err, restorer.ErrIncomplete) {
	// 			setPhaseConditionToPending(s.cb, &s.vmop.Status.Phase, vmopcondition.ReasonTODO, "waiting for the virtual machine will be stopped")
	// 			return &reconcile.Result{}, nil
	// 		}
	//
	// 		setPhaseConditionToFailed(s.cb, &s.vmop.Status.Phase, err)
	// 		return &reconcile.Result{}, err
	// 	}
	// }
	//
	// if s.vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
	// 	err := stopVirtualMachine(ctx, s.client, s.vmop.Spec.VirtualMachine, s.vmop.Namespace, "TODO")
	// 	if err != nil {
	// 		s.recorder.Event(
	// 			s.vmop,
	// 			corev1.EventTypeWarning,
	// 			virtv2.ReasonVMStopFailed,
	// 			err.Error(),
	// 		)
	// 	}
	//
	// 	s.vmop.Status.Phase = virtv2.VMOPPhaseCompleted
	// 	s.cb.Status(metav1.ConditionTrue).Reason(vmopcondition.ReasonTODO)
	//
	// 	return &reconcile.Result{}, nil
	// }

	return &reconcile.Result{}, nil
}
