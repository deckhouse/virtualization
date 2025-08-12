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

package snapshot

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/snapshot/step"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
)

type VMSnapshotRestore struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	vmop     *virtv2.VirtualMachineOperation
}

func NewVMSnapshotRestore(client client.Client, recorder eventrecord.EventRecorderLogger, vmop *virtv2.VirtualMachineOperation) *VMSnapshotRestore {
	return &VMSnapshotRestore{
		client:   client,
		recorder: recorder,
		vmop:     vmop,
	}
}

func (r VMSnapshotRestore) Sync(ctx context.Context, vm *virtv2.VirtualMachine) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmrestorecondition.VirtualMachineRestoreReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vm.Generation), &vm.Status.Conditions) }()

	return steptaker.NewStepTakers(
		step.NewDryRunStep(r.recorder, cb, vm),
		step.NewVMSnapshotReadyStep(r.client, r.recorder, cb, r.vmop),
		step.NewStopVMStep(r.client, r.recorder, cb, r.vmop),
		step.NewRestoreVMStep(r.client, r.recorder, cb, r.vmop),
		step.NewStartVMStep(r.client, r.recorder, cb, r.vmop),
	).Run(ctx, vm)
}
