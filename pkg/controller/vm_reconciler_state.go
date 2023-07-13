package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

type VMReconcilerState struct {
	Client    client.Client
	VM        *helper.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
	KVVM      *virtv1.VirtualMachine
	KVVMI     *virtv1.VirtualMachineInstance
	VMDByName map[string]*virtv2.VirtualMachineDisk
	// VMIByName map[string]*virtv2.VirtualMachineImage
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

func (state *VMReconcilerState) ApplySync(ctx context.Context, _ logr.Logger) error {
	if err := state.VM.UpdateMeta(ctx); err != nil {
		return fmt.Errorf("unable to update VM %q meta: %w", state.VM.Name(), err)
	}
	return nil
}

func (state *VMReconcilerState) ApplyUpdateStatus(ctx context.Context, _ logr.Logger) error {
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

func (state *VMReconcilerState) Reload(ctx context.Context, req reconcile.Request, log logr.Logger, _ client.Client) error {
	if err := state.VM.Fetch(ctx); err != nil {
		return fmt.Errorf("unable to get %q: %w", req.NamespacedName, err)
	}
	if state.VM.IsEmpty() {
		log.Info("Reconcile observe an absent VM: it may be deleted", "VM", req.NamespacedName)
		return nil
	}

	kvvmName := state.VM.Name()
	kvvm, err := helper.FetchObject(ctx, kvvmName, state.Client, &virtv1.VirtualMachine{})
	if err != nil {
		return fmt.Errorf("unable to get KubeVirt VM %q: %w", kvvmName, err)
	}
	state.KVVM = kvvm

	if state.KVVM != nil {
		if state.KVVM.Status.Created {
			// FIXME(VM): ObservedGeneration & DesiredGeneration only available since KubeVirt 1.0.0 which is only prereleased at the moment
			// FIXME(VM): Uncomment following check when KubeVirt updated to 1.0.0
			// if state.KVVM.Status.ObservedGeneration == state.KVVM.Status.DesiredGeneration {
			kvvmi, err := helper.FetchObject(ctx, kvvmName, state.Client, &virtv1.VirtualMachineInstance{})
			if err != nil {
				return fmt.Errorf("unable to get KubeVirt VMI %q: %w", kvvmName, err)
			}
			state.KVVMI = kvvmi
			//}
		}
	}

	var vmdByName map[string]*virtv2.VirtualMachineDisk

	for _, bd := range state.VM.Current().Spec.BlockDevices {
		switch bd.Type {
		case virtv2.ImageDevice:
			panic("NOT IMPLEMENTED")

		case virtv2.DiskDevice:
			vmd, err := helper.FetchObject(ctx, types.NamespacedName{
				Name:      bd.VirtualMachineDisk.Name,
				Namespace: state.VM.Name().Namespace,
			}, state.Client, &virtv2.VirtualMachineDisk{})
			if err != nil {
				return fmt.Errorf("unable to get VMD %q: %w", bd.VirtualMachineDisk.Name, err)
			}
			if vmd == nil {
				continue
			}
			if vmdByName == nil {
				vmdByName = make(map[string]*virtv2.VirtualMachineDisk)
			}
			vmdByName[bd.VirtualMachineDisk.Name] = vmd

		default:
			panic(fmt.Sprintf("unknown block device type %q", bd.Type))
		}
	}

	state.VMDByName = vmdByName

	return nil
}

func (state *VMReconcilerState) ShouldReconcile() bool {
	return !state.VM.IsEmpty()
}

func (state *VMReconcilerState) FindAttachedBlockDevice(spec virtv2.BlockDeviceSpec) *virtv2.BlockDeviceStatus {
	for i := range state.VM.Current().Status.BlockDevicesAttached {
		bda := &state.VM.Current().Status.BlockDevicesAttached[i]
		switch spec.Type {
		case virtv2.DiskDevice:
			if bda.Type == spec.Type && bda.VirtualMachineDisk.Name == spec.VirtualMachineDisk.Name {
				return bda
			}
		case virtv2.ImageDevice:
			panic("not implemented")
		default:
			panic(fmt.Sprintf("unexpected block device type %q", spec.Type))
		}
	}

	return nil
}

func (state *VMReconcilerState) CreateAttachedBlockDevice(spec virtv2.BlockDeviceSpec) *virtv2.BlockDeviceStatus {
	switch spec.Type {
	case virtv2.DiskDevice:
		vs := state.FindVolumeStatus(spec.VirtualMachineDisk.Name)
		if vs == nil {
			return nil
		}

		vmd, hasVmd := state.VMDByName[spec.VirtualMachineDisk.Name]
		if !hasVmd {
			return nil
		}

		return &virtv2.BlockDeviceStatus{
			Type:               virtv2.DiskDevice,
			VirtualMachineDisk: util.CopyByPointer(spec.VirtualMachineDisk),
			Target:             vs.Target,
			Size:               vmd.Status.Size,
		}

	case virtv2.ImageDevice:
		panic("not implemented")

	default:
		panic(fmt.Sprintf("unexpected block device type %q", spec.Type))
	}
}

func (state *VMReconcilerState) FindVolumeStatus(volumeName string) *virtv1.VolumeStatus {
	for i := range state.KVVMI.Status.VolumeStatus {
		vs := state.KVVMI.Status.VolumeStatus[i]
		if vs.Name == volumeName {
			return &vs
		}
	}
	return nil
}
