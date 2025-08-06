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

func (vmr VMSnapshotRestore) Sync(ctx context.Context, vm *virtv2.VirtualMachine) (reconcile.Result, error) {
	// cb := conditions.NewConditionBuilder(vmopcondition.Read).Generation(vm.Generation)
	// defer func() { conditions.SetCondition(cb, &vm.Status.Conditions) }()
	cb := conditions.NewConditionBuilder(vmrestorecondition.VirtualMachineRestoreReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vm.Generation), &vm.Status.Conditions) }()

	return steptaker.NewStepTakers(
		step.NewStopVMStep(vmr.recorder, cb),
	).Run(ctx, vm)
}
