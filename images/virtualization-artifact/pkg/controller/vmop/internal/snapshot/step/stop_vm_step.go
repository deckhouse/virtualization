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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
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
	if vm.Status.Phase == virtv2.MachineStopped {
		return nil, nil
	}

	if vm.Status.Phase == virtv2.MachineRunning {
		if !conditions.HasCondition(vmcondition.TypeMaintenance, vm.Status.Conditions) {
			conditions.SetCondition(
				conditions.NewConditionBuilder(vmcondition.TypeMaintenance).
					Generation(vm.GetGeneration()).
					Reason(vmcondition.ReasonMaintenanceRestore).
					Status(metav1.ConditionTrue).
					Message("VM is being moved to maintenance mode for restore operation"),
				&vm.Status.Conditions,
			)

			s.recorder.Event(s.vmop, "Normal", "MaintenanceMode", "VM is being moved to maintenance mode for restore operation")
		}

		return &reconcile.Result{}, nil
	}

	return nil, nil
}
