package controller

import (
	"context"
	"fmt"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMDReconcilerState struct {
	Client client.Client
	VMD    *helper.Resource[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus]
	DV     *cdiv1.DataVolume
	Result *reconcile.Result
}

func NewVMDReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client) *VMDReconcilerState {
	return &VMDReconcilerState{
		Client: client,
		VMD:    helper.NewResource[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus](name, log, client),
	}
}

func (state *VMDReconcilerState) ApplySync(ctx context.Context, log logr.Logger) error {
	if err := state.VMD.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VMD %q meta: %w", state.VMD.Name(), err)
	}
	return nil
}

func (state *VMDReconcilerState) ApplyUpdateStatus(ctx context.Context, log logr.Logger) error {
	return state.VMD.UpdateStatus(ctx)
}

func (state *VMDReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *VMDReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *VMDReconcilerState) ShouldApplyUpdateStatus() bool {
	return state.VMD.IsStatusChanged()
}

func (state *VMDReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	if err := state.VMD.Fetch(ctx, &virtv2.VirtualMachineDisk{}); err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if !state.VMD.IsFound() {
		log.Info("Reconcile observe an absent VMD: it may be deleted", "VMD", req.NamespacedName)
		return nil
	}

	var err error
	state.DV, err = helper.FetchObject(ctx, req.NamespacedName, client, &cdiv1.DataVolume{})
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	return nil
}

func (state *VMDReconcilerState) ShouldReconcile() bool {
	return state.VMD.IsFound()
}
