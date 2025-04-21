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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewRestartOperation(client client.Client, vmop *virtv2.VirtualMachineOperation) *RestartOperation {
	return &RestartOperation{
		client: client,
		vmop:   vmop,
	}
}

type RestartOperation struct {
	client client.Client
	vmop   *virtv2.VirtualMachineOperation
}

func (o RestartOperation) Do(ctx context.Context) error {
	kvvm := &virtv1.VirtualMachine{}
	err := o.client.Get(ctx, virtualMachineKeyByVmop(o.vmop), kvvm)
	if err != nil {
		return err
	}
	return kvvmutil.AddRestartAnnotation(ctx, o.client, kvvm)
}

func (o RestartOperation) Cancel(_ context.Context) (bool, error) {
	return false, nil
}

func (o RestartOperation) IsApplicableForVMPhase(phase virtv2.MachinePhase) bool {
	return phase == virtv2.MachineRunning ||
		phase == virtv2.MachineDegraded ||
		phase == virtv2.MachineStarting ||
		phase == virtv2.MachinePause
}

func (o RestartOperation) IsApplicableForRunPolicy(runPolicy virtv2.RunPolicy) bool {
	return runPolicy == virtv2.ManualPolicy ||
		runPolicy == virtv2.AlwaysOnUnlessStoppedManually ||
		runPolicy == virtv2.AlwaysOnPolicy
}

func (o RestartOperation) GetInProgressReason(_ context.Context) (vmopcondition.ReasonCompleted, error) {
	return vmopcondition.ReasonRestartInProgress, nil
}

func (o RestartOperation) IsFinalState() bool {
	return isFinalState(o.vmop)
}

func (o RestartOperation) IsComplete(ctx context.Context) (bool, string, error) {
	key := virtualMachineKeyByVmop(o.vmop)

	kvvmi := &virtv1.VirtualMachineInstance{}
	if err := o.client.Get(ctx, key, kvvmi); err != nil {
		return false, "", client.IgnoreNotFound(err)
	}

	vm := &virtv2.VirtualMachine{}
	if err := o.client.Get(ctx, key, kvvmi); err != nil {
		return false, "", client.IgnoreNotFound(err)
	}

	return kvvmi != nil && vm.Status.Phase == virtv2.MachineRunning &&
		isAfterSignalSentOrCreation(kvvmi.GetCreationTimestamp().Time, o.vmop), "", nil
}
