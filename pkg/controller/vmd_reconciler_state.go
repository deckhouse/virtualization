package controller

import (
	"context"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
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
	PV     *corev1.PersistentVolume
	Result *reconcile.Result
}

func NewVMDReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *VMDReconcilerState {
	return &VMDReconcilerState{
		Client: client,
		VMD: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachineDisk { return &virtv2.VirtualMachineDisk{} },
			func(obj *virtv2.VirtualMachineDisk) virtv2.VirtualMachineDiskStatus { return obj.Status },
		),
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
	if err := state.VMD.Fetch(ctx); err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if state.VMD.IsEmpty() {
		log.Info("Reconcile observe an absent VMD: it may be deleted", "VMD", req.NamespacedName)
		return nil
	}

	if dvName, hasKey := state.VMD.Current().Annotations[AnnVMDDataVolume]; hasKey {
		var err error
		name := types.NamespacedName{Name: dvName, Namespace: state.VMD.Current().Namespace}

		state.DV, err = helper.FetchObject(ctx, name, client, &cdiv1.DataVolume{})
		if err != nil {
			return fmt.Errorf("unable to get DV %q: %w", name, err)
		}
		if state.DV != nil {
			switch MapDataVolumePhaseToVMDPhase(state.DV.Status.Phase) {
			case virtv2.DiskProvisioning, virtv2.DiskReady:
				state.PVC, err = helper.FetchObject(ctx, name, client, &corev1.PersistentVolumeClaim{})
				if err != nil {
					return fmt.Errorf("unable to get PVC %q: %w", name, err)
				}
				if state.PVC == nil {
					return fmt.Errorf("no PVC %q found: expected existing PVC for DataVolume %q in phase %q", name, state.DV.Name, state.DV.Status.Phase)
				}
			}
		}
	}

	if state.PVC != nil {
		switch state.PVC.Status.Phase {
		case corev1.ClaimBound:
			pvName := state.PVC.Spec.VolumeName
			var err error
			state.PV, err = helper.FetchObject(ctx, types.NamespacedName{Name: pvName, Namespace: state.PVC.Namespace}, client, &corev1.PersistentVolume{})
			if err != nil {
				return fmt.Errorf("unable to get PV %q: %w", pvName, err)
			}
			if state.PV == nil {
				return fmt.Errorf("no PV %q found: expected existing PV for PVC %q in phase %q", pvName, state.PVC.Name, state.PVC.Status.Phase)
			}
		}
	}

	return nil
}

func (state *VMDReconcilerState) ShouldReconcile() bool {
	return !state.VMD.IsEmpty()
}
