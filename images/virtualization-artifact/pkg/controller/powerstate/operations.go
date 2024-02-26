package powerstate

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	kvv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

// StartVM starts VM via adding change request to the KVVM status.
func StartVM(ctx context.Context, cl client.Client, kvvm *kvv1.VirtualMachine) error {
	if kvvm == nil {
		return fmt.Errorf("kvvm must not be empty")
	}
	jp, err := BuildPatch(kvvm,
		kvv1.VirtualMachineStateChangeRequest{Action: kvv1.StartRequest})
	if err != nil {
		return err
	}
	return cl.Status().Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, jp), &client.SubResourcePatchOptions{})
}

// StopVM stops VM via deleting kvvmi.
// It implements force stop by immediately deleting VM's Pod.
func StopVM(ctx context.Context, cl client.Client, kvvmi *kvv1.VirtualMachineInstance, force bool) error {
	if kvvmi == nil {
		return fmt.Errorf("kvvmi must not be empty")
	}
	if err := cl.Delete(ctx, kvvmi, &client.DeleteOptions{}); err != nil {
		return err
	}
	if force {
		return kvvmutil.DeletePodByKVVMI(ctx, cl, kvvmi, &client.DeleteOptions{GracePeriodSeconds: util.GetPointer(int64(0))})
	}
	return nil
}

// RestartVM restarts VM via adding stop and start change requests to the KVVM status.
// It implements force stop by immediately deleting VM's Pod.
func RestartVM(ctx context.Context, cl client.Client, kvvm *kvv1.VirtualMachine, kvvmi *kvv1.VirtualMachineInstance, force bool) error {
	if kvvm == nil {
		return fmt.Errorf("kvvm must not be empty")
	}
	if kvvmi == nil {
		return fmt.Errorf("kvvmi must not be empty")
	}

	jp, err := BuildPatch(kvvm,
		kvv1.VirtualMachineStateChangeRequest{Action: kvv1.StopRequest, UID: &kvvmi.UID},
		kvv1.VirtualMachineStateChangeRequest{Action: kvv1.StartRequest})
	if err != nil {
		return err
	}

	err = cl.Status().Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, jp), &client.SubResourcePatchOptions{})
	if err != nil {
		return err
	}
	if force {
		return kvvmutil.DeletePodByKVVMI(ctx, cl, kvvmi, &client.DeleteOptions{GracePeriodSeconds: util.GetPointer(int64(0))})
	}
	return nil
}

// SafeRestartVM restarts VM via adding stop and start change requests to the KVVM status if no other requests are in progress.
func SafeRestartVM(ctx context.Context, cl client.Client, kvvm *kvv1.VirtualMachine, kvvmi *kvv1.VirtualMachineInstance) error {
	if kvvm == nil {
		return fmt.Errorf("kvvm must not be empty")
	}
	if kvvmi == nil {
		return fmt.Errorf("kvvmi must not be empty")
	}

	jp, err := BuildPatchSafeRestart(kvvm, kvvmi)
	if err != nil {
		return err
	}
	if jp == nil {
		return nil
	}
	return cl.Status().Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, jp), &client.SubResourcePatchOptions{})
}
