package cpu

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

	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type VMCPUReconcilerState struct {
	Client client.Client
	VMCPU  *helper.Resource[*virtv2.VirtualMachineCPUModel, virtv2.VirtualMachineCPUModelStatus]

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
		log.Info("Reconcile observe an absent VMCPU: it may be deleted", "VMCPU.name", req.NamespacedName)
		return nil
	}

	vmKey := types.NamespacedName{Name: state.VMCPU.Current().Spec.VMName, Namespace: state.VMCPU.Current().Namespace}
	state.VM, err = helper.FetchObject(ctx, vmKey, client, &virtv2.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("unable to get VM %s: %w", vmKey, err)
	}

	kvvmiKey := types.NamespacedName{Name: state.VMCPU.Current().Spec.VMName, Namespace: state.VMCPU.Current().Namespace}
	state.KVVMI, err = helper.FetchObject(ctx, kvvmiKey, client, &virtv1.VirtualMachineInstance{})
	if err != nil {
		return fmt.Errorf("unable to get KVVMI %s: %w", kvvmiKey, err)
	}

	switch state.VMCPU.Current().Spec.BlockDevice.Type {
	case virtv2.BlockDeviceAttachmentTypeVirtualMachineDisk:
		vmdKey := types.NamespacedName{Name: state.VMCPU.Current().Spec.BlockDevice.VirtualMachineDisk.Name, Namespace: state.VMCPU.Current().Namespace}
		state.VMD, err = helper.FetchObject(ctx, vmdKey, client, &virtv2.VirtualMachineDisk{})
		if err != nil {
			return fmt.Errorf("unable to get VMD %s: %w", vmdKey, err)
		}

		if state.VMD == nil {
			return nil
		}

		pvcKey := types.NamespacedName{Name: state.VMD.Status.Target.PersistentVolumeClaimName, Namespace: state.VMCPU.Current().Namespace}
		state.PVC, err = helper.FetchObject(ctx, pvcKey, client, &corev1.PersistentVolumeClaim{})
		if err != nil {
			return fmt.Errorf("unable to get PVC %s: %w", pvcKey, err)
		}
	default:
		return fmt.Errorf("unknown block device attachment type %s", state.VMCPU.Current().Spec.VMName)
	}

	return nil
}

func (state *VMCPUReconcilerState) ShouldReconcile(_ logr.Logger) bool {
	return !state.VMCPU.IsEmpty()
}

func (state *VMCPUReconcilerState) isDeletion() bool {
	return state.VMCPU.Current().DeletionTimestamp != nil
}

func (state *VMCPUReconcilerState) IndexVMStatusBDA() int {
	if state.VM == nil || state.VMD == nil {
		return -1
	}

	for i, blockDevice := range state.VM.Status.BlockDevicesAttached {
		if blockDevice.VirtualMachineDisk != nil && blockDevice.VirtualMachineDisk.Name == state.VMD.Name {
			return i
		}
	}
	return -1
}

// RemoveVMStatusBDA removes device from VM.Status.BlockDevicesAttached by its name.
func (state *VMCPUReconcilerState) RemoveVMStatusBDA() bool {
	if state.VM == nil {
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
