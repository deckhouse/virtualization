/*
Copyright 2025 Flant JSC

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

package service

import (
	"context"

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewStopOperation(client client.Client, vmop *v1alpha2.VirtualMachineOperation) *StopOperation {
	return &StopOperation{
		client: client,
		vmop:   vmop,
	}
}

type StopOperation struct {
	client client.Client
	vmop   *v1alpha2.VirtualMachineOperation
}

func (o StopOperation) Execute(ctx context.Context) error {
	kvvmi := &virtv1.VirtualMachineInstance{}
	err := o.client.Get(ctx, virtualMachineKeyByVmop(o.vmop), kvvmi)
	if err != nil {
		return err
	}
	return powerstate.StopVM(ctx, o.client, kvvmi, o.vmop.Spec.Force)
}

func (o StopOperation) IsApplicableForVMPhase(phase v1alpha2.MachinePhase) bool {
	return phase == v1alpha2.MachineRunning ||
		phase == v1alpha2.MachineDegraded ||
		phase == v1alpha2.MachineStarting ||
		phase == v1alpha2.MachinePause ||
		phase == v1alpha2.MachinePending
}

func (o StopOperation) IsApplicableForRunPolicy(runPolicy v1alpha2.RunPolicy) bool {
	return runPolicy == v1alpha2.ManualPolicy || runPolicy == v1alpha2.AlwaysOnUnlessStoppedManually
}

func (o StopOperation) GetInProgressReason() vmopcondition.ReasonCompleted {
	return vmopcondition.ReasonStopInProgress
}

func (o StopOperation) IsComplete(ctx context.Context) (bool, string, error) {
	vm := &v1alpha2.VirtualMachine{}
	err := o.client.Get(ctx, client.ObjectKey{Namespace: o.vmop.Namespace, Name: o.vmop.Spec.VirtualMachine}, vm)
	if err != nil {
		return false, "", client.IgnoreNotFound(err)
	}

	return vm.Status.Phase == v1alpha2.MachineStopped, "", nil
}
