package controller

import (
	"context"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/go-logr/logr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMDReconcilerState struct {
	Client client.Client

	VMD        *virtv2.VirtualMachineDisk
	VMDMutated *virtv2.VirtualMachineDisk
	DV         *cdiv1.DataVolume
	Result     *reconcile.Result
}

func NewVMDReconcilerState(client client.Client) *VMDReconcilerState {
	return &VMDReconcilerState{
		Client: client,
	}
}

// TODO: generics to generate parts of these methods especially resource-objects-related

func (state *VMDReconcilerState) ApplySync(ctx context.Context, log logr.Logger) error {
	if state.VMD == nil || state.VMDMutated == nil {
		return nil
	}
	if !reflect.DeepEqual(state.VMD.ObjectMeta, state.VMDMutated.ObjectMeta) {
		if !reflect.DeepEqual(state.VMD.Status, state.VMDMutated.Status) {
			return fmt.Errorf("status update is not allowed in sync phase")
		}

		if err := state.Client.Update(ctx, state.VMDMutated); err != nil {
			log.Error(err, "Unable to sync update VMD meta", "name", state.VMDMutated.Name)
			return err
		}
	}
	return nil
}

// TODO: generic methods implementing following checks
func (state *VMDReconcilerState) ApplyUpdateStatus(ctx context.Context, log logr.Logger) error {
	if !reflect.DeepEqual(state.VMD.ObjectMeta, state.VMDMutated.ObjectMeta) {
		return fmt.Errorf("meta update is not allowed in updateStatus phase")
	}

	if !reflect.DeepEqual(state.VMD.Status, state.VMDMutated.Status) {
		if err := state.Client.Status().Update(ctx, state.VMDMutated); err != nil {
			log.Error(err, "unable to update VMD status", "name", state.VMDMutated.Name)
			return err
		}
	}
	return nil
}

func (state *VMDReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *VMDReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *VMDReconcilerState) ShouldApplyUpdateStatus() bool {
	return !reflect.DeepEqual(state.VMD.Status, state.VMDMutated.Status)
}

func (state *VMDReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	vmd, err := FetchObject(ctx, req.NamespacedName, client, &virtv2.VirtualMachineDisk{})
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if vmd == nil {
		log.Info("Reconcile observe absent VMD: it may be deleted", "VMD", req.NamespacedName)
		return nil
	}
	state.VMD = vmd
	state.VMDMutated = vmd.DeepCopy()

	state.DV, err = FetchObject(ctx, req.NamespacedName, client, &cdiv1.DataVolume{})
	if err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	return nil
}

func (state *VMDReconcilerState) ShouldReconcile() bool {
	return state.VMD != nil
}
