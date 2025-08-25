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

	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewRestartOperation(client client.Client, vmop *v1alpha2.VirtualMachineOperation) *RestartOperation {
	return &RestartOperation{
		client: client,
		vmop:   vmop,
	}
}

type RestartOperation struct {
	client client.Client
	vmop   *v1alpha2.VirtualMachineOperation
}

func (o RestartOperation) Execute(ctx context.Context) error {
	kvvm := &virtv1.VirtualMachine{}
	err := o.client.Get(ctx, virtualMachineKeyByVmop(o.vmop), kvvm)
	if err != nil {
		return err
	}
	return kvvmutil.AddRestartAnnotation(ctx, o.client, kvvm)
}

func (o RestartOperation) IsApplicableForVMPhase(phase v1alpha2.MachinePhase) bool {
	return phase == v1alpha2.MachineRunning ||
		phase == v1alpha2.MachineDegraded ||
		phase == v1alpha2.MachineStarting ||
		phase == v1alpha2.MachinePause
}

func (o RestartOperation) IsApplicableForRunPolicy(runPolicy v1alpha2.RunPolicy) bool {
	return runPolicy == v1alpha2.ManualPolicy ||
		runPolicy == v1alpha2.AlwaysOnUnlessStoppedManually ||
		runPolicy == v1alpha2.AlwaysOnPolicy
}

func (o RestartOperation) GetInProgressReason() vmopcondition.ReasonCompleted {
	return vmopcondition.ReasonRestartInProgress
}

func (o RestartOperation) IsComplete(ctx context.Context) (bool, string, error) {
	key := virtualMachineKeyByVmop(o.vmop)

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := o.client.Get(ctx, key, kvvmi); err != nil {
		return false, "", client.IgnoreNotFound(err)
	}

	vm := &v1alpha2.VirtualMachine{}
	if err := o.client.Get(ctx, key, vm); err != nil {
		return false, "", err
	}

	return kvvmi != nil && vm.Status.Phase == v1alpha2.MachineRunning &&
		genericservice.IsAfterSignalSentOrCreation(kvvmi.GetCreationTimestamp().Time, o.vmop), "", nil
}
