package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type VMBDAReconcilerState struct {
	Client client.Client
	VMBDA  *helper.Resource[*virtv2.VirtualMachineBlockDeviceAttachment, virtv2.VirtualMachineBlockDeviceAttachmentStatus]
	VM     *virtv2.VirtualMachine
	KVVMI  *virtv1.VirtualMachineInstance

	VMD *virtv2.VirtualMachineDisk
	PVC *corev1.PersistentVolumeClaim

	Result *reconcile.Result

	FailureReason  string
	FailureMessage string
}

func NewVMBDAReconcilerState(name types.NamespacedName, log logr.Logger, client client.Client, cache cache.Cache) *VMBDAReconcilerState {
	state := &VMBDAReconcilerState{
		Client: client,
		VMBDA: helper.NewResource(
			name, log, client, cache,
			func() *virtv2.VirtualMachineBlockDeviceAttachment {
				return &virtv2.VirtualMachineBlockDeviceAttachment{}
			},
			func(obj *virtv2.VirtualMachineBlockDeviceAttachment) virtv2.VirtualMachineBlockDeviceAttachmentStatus {
				return obj.Status
			},
		),
	}

	return state
}

func (state *VMBDAReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.VMBDA.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VMBDA %q meta: %w", state.VMBDA.Name(), err)
	}
	return nil
}

func (state *VMBDAReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
	return state.VMBDA.UpdateStatus(ctx)
}

func (state *VMBDAReconcilerState) SetReconcilerResult(result *reconcile.Result) {
	state.Result = result
}

func (state *VMBDAReconcilerState) GetReconcilerResult() *reconcile.Result {
	return state.Result
}

func (state *VMBDAReconcilerState) SetStatusFailure(reason, message string) {
	state.FailureReason = reason
	state.FailureMessage = message
}

func (state *VMBDAReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error {
	err := state.VMBDA.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("unable to get VMBDA %s: %w", req.NamespacedName, err)
	}

	if state.VMBDA.IsEmpty() {
		log.Info("Reconcile observe an absent VMBDA: it may be deleted", "vmbda.name", req.NamespacedName)
		return nil
	}

	vmKey := types.NamespacedName{Name: state.VMBDA.Current().Spec.VMName, Namespace: state.VMBDA.Current().Namespace}
	state.VM, err = helper.FetchObject(ctx, vmKey, client, &virtv2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("unable to get VM %s: %w", vmKey, err)
	}

	kvvmiKey := types.NamespacedName{Name: state.VMBDA.Current().Spec.VMName, Namespace: state.VMBDA.Current().Namespace}
	state.KVVMI, err = helper.FetchObject(ctx, kvvmiKey, client, &virtv1.VirtualMachineInstance{})
	if err != nil {
		return fmt.Errorf("unable to get KVVMI %s: %w", kvvmiKey, err)
	}

	switch state.VMBDA.Current().Spec.BlockDevice.Type {
	case virtv2.BlockDeviceAttachmentTypeVirtualMachineDisk:
		vmdKey := types.NamespacedName{Name: state.VMBDA.Current().Spec.BlockDevice.VirtualMachineDisk.Name, Namespace: state.VMBDA.Current().Namespace}
		state.VMD, err = helper.FetchObject(ctx, vmdKey, client, &virtv2.VirtualMachineDisk{})
		if err != nil {
			return fmt.Errorf("unable to get VMD %s: %w", vmdKey, err)
		}

		if state.VMD == nil {
			return nil
		}

		pvcKey := types.NamespacedName{Name: state.VMD.Status.Target.PersistentVolumeClaimName, Namespace: state.VMBDA.Current().Namespace}
		state.PVC, err = helper.FetchObject(ctx, pvcKey, client, &corev1.PersistentVolumeClaim{})
		if err != nil {
			return fmt.Errorf("unable to get PVC %s: %w", pvcKey, err)
		}
	default:
		return fmt.Errorf("unknown block device attachment type %s", state.VMBDA.Current().Spec.VMName)
	}

	return nil
}

func (state *VMBDAReconcilerState) ShouldReconcile(_ logr.Logger) bool {
	return !state.VMBDA.IsEmpty()
}

func (state *VMBDAReconcilerState) isDeletion() bool {
	return state.VMBDA.Current().DeletionTimestamp != nil
}

func (state *VMBDAReconcilerState) IndexVMStatusBDA() int {
	if state.VM == nil {
		return -1
	}

	for i, blockDevice := range state.VM.Status.BlockDevicesAttached {
		if blockDevice.VirtualMachineDisk != nil && blockDevice.VirtualMachineDisk.Name == state.VMD.Name {
			return i
		}
	}
	return -1
}

// RemoveVMStatusBDA removes device from VM.Status.BlockDevicesAttached
// by its name.
func (state *VMBDAReconcilerState) RemoveVMStatusBDA() bool {
	if state.VM == nil {
		return false
	}
	if state.VMD == nil {
		return false
	}

	blockDeviceIndex := state.IndexVMStatusBDA()
	if blockDeviceIndex == -1 {
		return false
	}

	state.VM.Status.BlockDevicesAttached = append(
		state.VM.Status.BlockDevicesAttached[:blockDeviceIndex],
		state.VM.Status.BlockDevicesAttached[blockDeviceIndex+1:]...,
	)
	return true
}
