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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
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
	key := virtualMachineKeyByVmop(o.vmop)

	kvvmi := &virtv1.VirtualMachineInstance{}
	err := o.client.Get(ctx, key, kvvmi)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// The instance is deleted directly, bypassing the power state handler of the virtual machine
	// controller, so the run strategy has to be settled here. An internal virtual machine still
	// carrying the create-time RunStrategyAlways would have KubeVirt recreate the instance right
	// back and the stop would be silently undone. That strategy stays in place until the instance
	// leaves Pending (see instanceLeftPending in vm/internal/sync_power_state.go), which is
	// exactly the window a stop of a still-starting machine lands in.
	//
	// Order matters: patching first means a failed patch leaves the instance alone and the
	// operation simply retries, whereas stopping first would open a window with the instance
	// already gone and RunStrategyAlways still in place.
	kvvm := &virtv1.VirtualMachine{}
	if err = o.client.Get(ctx, key, kvvm); err != nil {
		return err
	}
	if err = kvvmutil.EnsureRunStrategy(ctx, o.client, kvvm, virtv1.RunStrategyManual); err != nil {
		return err
	}

	return powerstate.StopVM(ctx, o.client, kvvmi, o.vmop.Spec.Force)
}

func (o StopOperation) IsApplicableForVMPhase(phase v1alpha2.MachinePhase) bool {
	return phase == v1alpha2.MachineRunning ||
		phase == v1alpha2.MachineDegraded ||
		phase == v1alpha2.MachineStarting ||
		phase == v1alpha2.MachinePause ||
		phase == v1alpha2.MachinePending ||
		phase == v1alpha2.MachineStopping && isForceRequested(o.vmop)
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
