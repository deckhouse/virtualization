package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type VMReconcilerState struct {
	Client client.Client
	VM     *helper.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
	Result *reconcile.Result
}

func NewVMReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *VMReconcilerState {
	return &VMReconcilerState{
		Client: client,
		VM: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachine { return &virtv2.VirtualMachine{} },
			func(obj *virtv2.VirtualMachine) virtv2.VirtualMachineStatus { return obj.Status },
		),
	}
}

// ApplySync
//
// TODO replace arg names with _ or use them in code and remove nolint comment
//
//nolint:revive
func (state *VMReconcilerState) ApplySync(ctx context.Context, log logr.Logger) error {
	if err := state.VM.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VM %q meta: %w", state.VM.Name(), err)
	}
	return nil
}

// ApplyUpdateStatus
//
// TODO replace arg names with _ or use them in code and remove nolint comment
//
//nolint:revive
func (state *VMReconcilerState) ApplyUpdateStatus(ctx context.Context, log logr.Logger) error {
	return state.VM.UpdateStatus(ctx)
}

func (state *VMReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *VMReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *VMReconcilerState) ShouldApplyUpdateStatus() bool {
	return state.VM.IsStatusChanged()
}

// Reload
//
// TODO replace arg names with _ or use them in code and remove nolint comment
//
//nolint:revive
func (state *VMReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	if err := state.VM.Fetch(ctx); err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if state.VM.IsEmpty() {
		log.Info("Reconcile observe an absent VM: it may be deleted", "VM", req.NamespacedName)
		return nil
	}
	return nil
}

func (state *VMReconcilerState) ShouldReconcile() bool {
	return !state.VM.IsEmpty()
}
