package controller

import (
	"context"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMDReconcilerState struct {
	Client client.Client
	VMD    *helper.Resource[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus]
	DV     *cdiv1.DataVolume
	PVC    *corev1.PersistentVolumeClaim
	Result *reconcile.Result

	PersistentVolumeClaimName types.NamespacedName
}

func NewVMDReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *VMDReconcilerState {
	return &VMDReconcilerState{
		Client: client,
		VMD:    helper.NewResource[*virtv2.VirtualMachineDisk, virtv2.VirtualMachineDiskStatus](name, log, client, cache, &virtv2.VirtualMachineDisk{}),
	}
}

func (state *VMDReconcilerState) ApplySync(ctx context.Context, log logr.Logger) error {
	if err := state.VMD.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VMD %q meta: %w", state.VMD.Name(), err)
	}
	return nil
}

func (state *VMDReconcilerState) ApplyUpdateStatus(ctx context.Context, log logr.Logger) error {
	log.V(2).Info("VMDReconcilerState.ApplyUpdateStatus before", "VMD.Status", state.VMD.Changed().Status, "state.PersistentVolumeClaimName", state.PersistentVolumeClaimName)
	if err := state.VMD.UpdateStatus(ctx); err != nil {
		return err
	}
	log.V(2).Info("VMDReconcilerState.ApplyUpdateStatus after", "VMD.Status", state.VMD.Changed().Status, "state.PersistentVolumeClaimName", state.PersistentVolumeClaimName)

	// FIXME: remove after some testing
	//{
	//	obj := &virtv2.VirtualMachineDisk{}
	//	err := state.Client.Get(ctx, state.VMD.Name(), obj)
	//	log.V(2).Info("EXTRA GET", "err", err, "obj", obj, "status", obj.Status)
	//
	//	time.Sleep(10 * time.Second)
	//	obj2 := &virtv2.VirtualMachineDisk{}
	//	err = state.Client.Get(ctx, state.VMD.Name(), obj2)
	//	log.V(2).Info("EXTRA GET 2", "err", err, "obj", obj2, "status", obj2.Status)
	//}
	//
	//time.Sleep(60 * time.Second)
	return nil
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
	if err := state.VMD.Fetch(ctx); err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if state.VMD.IsEmpty() {
		log.Info("Reconcile observe an absent VMD: it may be deleted", "VMD", req.NamespacedName)
		return nil
	}

	log.V(2).Info("VMD State Reload", "status pvc", state.VMD.Current().Status.PersistentVolumeClaimName, "state pvc", state.PersistentVolumeClaimName, "isEmpty", util.IsEmpty(state.PersistentVolumeClaimName))

	if state.VMD.Current().Status.PersistentVolumeClaimName != "" {
		log.V(2).Info("VMD State Reload", "restore pvc name", state.VMD.Current().Status.PersistentVolumeClaimName)

		state.PersistentVolumeClaimName = types.NamespacedName{
			Name:      state.VMD.Current().Status.PersistentVolumeClaimName,
			Namespace: req.Namespace,
		}

		var err error
		state.DV, err = helper.FetchObject(ctx, state.PersistentVolumeClaimName, client, &cdiv1.DataVolume{})
		if err != nil {
			return fmt.Errorf("unable to get DV %q: %w", state.PersistentVolumeClaimName, err)
		}

		// FIXME: This is happening: dv is nil here, why?? Answer: probably because of client cache.
		log.V(2).Info("VMD State Reload", "DV", state.DV)

		state.PVC, err = helper.FetchObject(ctx, state.PersistentVolumeClaimName, client, &corev1.PersistentVolumeClaim{})
		if err != nil {
			return fmt.Errorf("unable to get PVC %q: %w", state.PersistentVolumeClaimName, err)
		}
	}

	return nil
}

func (state *VMDReconcilerState) ShouldReconcile() bool {
	return !state.VMD.IsEmpty()
}
