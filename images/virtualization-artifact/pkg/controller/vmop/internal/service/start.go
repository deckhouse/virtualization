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

func NewStartOperation(client client.Client, vmop *virtv2.VirtualMachineOperation) *StartOperation {
	return &StartOperation{
		client: client,
		vmop:   vmop,
	}
}

type StartOperation struct {
	client client.Client
	vmop   *virtv2.VirtualMachineOperation
}

func (o StartOperation) Do(ctx context.Context) error {
	kvvm := &virtv1.VirtualMachine{}
	err := o.client.Get(ctx, virtualMachineKeyByVmop(o.vmop), kvvm)
	if err != nil {
		return err
	}
	return kvvmutil.AddStartAnnotation(ctx, o.client, kvvm)
}

func (o StartOperation) Cancel(_ context.Context) (bool, error) {
	return false, nil
}

func (o StartOperation) IsApplicableForVMPhase(phase virtv2.MachinePhase) bool {
	return phase == virtv2.MachineStopped || phase == virtv2.MachineStopping

}

func (o StartOperation) IsApplicableForRunPolicy(runPolicy virtv2.RunPolicy) bool {
	return runPolicy == virtv2.ManualPolicy || runPolicy == virtv2.AlwaysOnUnlessStoppedManually
}

func (o StartOperation) GetInProgressReason(_ context.Context) (vmopcondition.ReasonCompleted, error) {
	return vmopcondition.ReasonStartInProgress, nil
}

func (o StartOperation) IsFinalState() bool {
	return isFinalState(o.vmop)
}

func (o StartOperation) IsComplete(ctx context.Context) (bool, string, error) {
	vm := &virtv2.VirtualMachine{}
	err := o.client.Get(ctx, client.ObjectKey{Namespace: o.vmop.Namespace, Name: o.vmop.Spec.VirtualMachine}, vm)
	if err != nil {
		return false, "", client.IgnoreNotFound(err)
	}

	return vm.Status.Phase == virtv2.MachineRunning, "", nil
}
