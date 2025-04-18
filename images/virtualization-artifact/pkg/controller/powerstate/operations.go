/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package powerstate

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	kvv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
)

// StartVM starts VM via adding change request to the KVVM status.
func StartVM(ctx context.Context, cl client.Client, kvvm *kvv1.VirtualMachine) error {
	if kvvm == nil {
		return fmt.Errorf("kvvm must not be empty")
	}
	jp, err := BuildPatch(kvvm,
		kvv1.VirtualMachineStateChangeRequest{Action: kvv1.StartRequest})
	if err != nil {
		if errors.Is(err, ErrChangesAlreadyExist) {
			return nil
		}
		return err
	}
	return cl.Status().Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, jp), &client.SubResourcePatchOptions{})
}

// StopVM stops VM via deleting kvvmi.
// It implements force stop by immediately deleting VM's Pod.
func StopVM(ctx context.Context, cl client.Client, kvvmi *kvv1.VirtualMachineInstance, force *bool) error {
	if kvvmi == nil {
		return fmt.Errorf("kvvmi must not be empty")
	}
	if err := cl.Delete(ctx, kvvmi, &client.DeleteOptions{}); err != nil {
		return err
	}
	if force != nil && *force {
		return kvvmutil.DeletePodByKVVMI(ctx, cl, kvvmi, &client.DeleteOptions{GracePeriodSeconds: pointer.GetPointer(int64(0))})
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
		if errors.Is(err, ErrChangesAlreadyExist) {
			return nil
		}
		return err
	}

	err = cl.Status().Patch(ctx, kvvm, client.RawPatch(types.JSONPatchType, jp), &client.SubResourcePatchOptions{})
	if err != nil {
		return err
	}
	if force {
		return kvvmutil.DeletePodByKVVMI(ctx, cl, kvvmi, &client.DeleteOptions{GracePeriodSeconds: pointer.GetPointer(int64(0))})
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
