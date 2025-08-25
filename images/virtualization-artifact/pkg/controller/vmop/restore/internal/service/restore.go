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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/snapshot"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func NewRestoreOperation(client client.Client, recorder eventrecord.EventRecorderLogger, vmop *virtv2.VirtualMachineOperation) *RestoreOperation {
	return &RestoreOperation{
		client:   client,
		vmop:     vmop,
		recorder: recorder,
		restore:  snapshot.NewVMSnapshotRestore(client, recorder, vmop),
	}
}

type RestoreOperation struct {
	client   client.Client
	vmop     *virtv2.VirtualMachineOperation
	restore  *snapshot.VMSnapshotRestore
	recorder eventrecord.EventRecorderLogger
}

func (o RestoreOperation) Execute(ctx context.Context) (reconcile.Result, error) {
	vm := &virtv2.VirtualMachine{}
	err := o.client.Get(ctx, virtualMachineKeyByVmop(o.vmop), vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	return o.restore.Sync(ctx, vm)
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

func (o RestoreOperation) GetInProgressReason() vmopcondition.ReasonCompleted {
	return vmopcondition.ReasonRestoreInProgress
}

func (o RestoreOperation) IsFinalState() bool {
	// return isFinalState(o.vmop)
	return false
}

func (o RestoreOperation) IsComplete(ctx context.Context) (bool, string, error) {
	c, ok := conditions.GetCondition(vmopcondition.TypeRestoreCompleted, o.vmop.Status.Conditions)
	return ok && c.Status == metav1.ConditionTrue, "", nil
}
