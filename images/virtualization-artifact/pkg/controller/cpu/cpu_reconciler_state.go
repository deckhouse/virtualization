package cpu

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmattachee"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMCPUReconcilerState struct {
	*vmattachee.AttacheeState[*virtv2.VirtualMachineCPUModel, virtv2.VirtualMachineCPUModelStatus]

	Client client.Client
	VMCPU  *helper.Resource[*virtv2.VirtualMachineCPUModel, virtv2.VirtualMachineCPUModelStatus]
	Nodes  []v1.Node

	Result *reconcile.Result
}

func NewVMCPUReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *VMCPUReconcilerState {
	state := &VMCPUReconcilerState{
		Client: client,
		VMCPU: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachineCPUModel {
				return &virtv2.VirtualMachineCPUModel{}
			},
			func(obj *virtv2.VirtualMachineCPUModel) virtv2.VirtualMachineCPUModelStatus {
				return obj.Status
			},
		),
	}

	state.AttacheeState = vmattachee.NewAttacheeState(
		state,
		virtv2.FinalizerVMCPUProtection,
		state.VMCPU,
	)

	return state
}

func (state *VMCPUReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.VMCPU.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VMCPU %q meta: %w", state.VMCPU.Name(), err)
	}
	return nil
}

func (state *VMCPUReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.VMCPU.UpdateStatus(ctx)
}

func (state *VMCPUReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *VMCPUReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *VMCPUReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	err := state.VMCPU.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get VMCPU %s: %w", req.NamespacedName, err)
	}

	if state.VMCPU.IsEmpty() {
		log.Info("Reconcile observe an absent VMCPU: it may be deleted", "vmcpu.name", req.NamespacedName)
		return nil
	}

	var nodes v1.NodeList
	err = client.List(ctx, &nodes)
	if err != nil {
		return err
	}

	state.Nodes = nodes.Items

	return state.AttacheeState.Reload(ctx, req, log, client)
}

func (state *VMCPUReconcilerState) ShouldReconcile(log logr.Logger) bool {
	if state.VMCPU.IsEmpty() {
		return false
	}

	if state.AttacheeState.ShouldReconcile(log) {
		return true
	}

	return true
}

func (state *VMCPUReconcilerState) IsAttachedToVM(vm virtv2.VirtualMachine) bool {
	if state.VMCPU.IsEmpty() {
		return false
	}

	return state.VMCPU.Name().Name == vm.Spec.CPU.ModelName
}

func (state *VMCPUReconcilerState) isDeletion() bool {
	return !state.VMCPU.Current().ObjectMeta.DeletionTimestamp.IsZero()
}
