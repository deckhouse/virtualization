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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewRestoreOperation(client client.Client, vmop *virtv2.VirtualMachineOperation) *RestoreOperation {
	return &RestoreOperation{
		client: client,
		vmop:   vmop,
	}
}

type RestoreOperation struct {
	client client.Client
	vmop   *virtv2.VirtualMachineOperation
}

func (o RestoreOperation) Do(ctx context.Context) error {
	kvvm := &virtv1.VirtualMachine{}
	err := o.client.Get(ctx, virtualMachineKeyByVmop(o.vmop), kvvm)
	if err != nil {
		return err
	}
	// return kvvmutil.AddRestoreAnnotation(ctx, o.client, kvvm)
	return nil
}

func (o RestoreOperation) Cancel(_ context.Context) (bool, error) {
	return false, nil
}

func (o RestoreOperation) IsApplicableForVMPhase(phase virtv2.MachinePhase) bool {
	return phase == virtv2.MachineStopped || phase == virtv2.MachineRunning || phase == virtv2.MachinePending
}

func (o RestoreOperation) IsApplicableForRunPolicy(runPolicy virtv2.RunPolicy) bool {
	return runPolicy == virtv2.ManualPolicy || runPolicy == virtv2.AlwaysOnUnlessStoppedManually || runPolicy == virtv2.AlwaysOffPolicy
}

func (o RestoreOperation) GetInProgressReason(_ context.Context) (vmopcondition.ReasonCompleted, error) {
	return vmopcondition.ReasonRestoreInProgress, nil
}

func (o RestoreOperation) IsFinalState() bool {
	return isFinalState(o.vmop)
}

func (o RestoreOperation) IsComplete(ctx context.Context) (bool, string, error) {
	vm := &virtv2.VirtualMachine{}
	err := o.client.Get(ctx, client.ObjectKey{Namespace: o.vmop.Namespace, Name: o.vmop.Spec.VirtualMachine}, vm)
	if err != nil {
		return false, "", client.IgnoreNotFound(err)
	}

	return vm.Status.Phase == virtv2.MachineRunning, "", nil
}
