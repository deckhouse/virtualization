/*
Copyright 2026 Flant JSC

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

package validators

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// PeerMigrationLockValidator keeps the spec of a VM that moves to a peer cluster, or arrives from one,
// as it is until the move ends: the peer received that spec. Releasing the lock together with a change is
// allowed: that is how the move hands the VM back.
type PeerMigrationLockValidator struct {
	client client.Client
}

func NewPeerMigrationLockValidator(c client.Client) *PeerMigrationLockValidator {
	return &PeerMigrationLockValidator{client: c}
}

func (v *PeerMigrationLockValidator) ValidateCreate(_ context.Context, _ *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, nil
}

func (v *PeerMigrationLockValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if err := validateLockOwner(ctx, oldVM, newVM); err != nil {
		return nil, err
	}
	if equality.Semantic.DeepEqual(oldVM.Spec, newVM.Spec) {
		return nil, nil
	}
	if op := oldVM.GetAnnotations()[annotations.AnnVMMovedToPeer]; op != "" && newVM.GetAnnotations()[annotations.AnnVMMovedToPeer] == op {
		var vmop v1alpha2.VirtualMachineOperation
		if err := v.client.Get(ctx, types.NamespacedName{Name: op, Namespace: oldVM.Namespace}, &vmop); err == nil && commonvmop.PeerName(&vmop) != "" {
			return nil, fmt.Errorf("the virtual machine is being migrated to cluster %q. Cancel the migration by deleting the VirtualMachineOperation first", commonvmop.PeerName(&vmop))
		}
		return nil, errors.New("the virtual machine is being migrated to a peer cluster. Cancel the migration by deleting the VirtualMachineOperation first")
	}
	if uid := oldVM.GetAnnotations()[annotations.AnnVMMigrationReceiver]; uid != "" && newVM.GetAnnotations()[annotations.AnnVMMigrationReceiver] == uid {
		return nil, errors.New("the virtual machine is being migrated from a peer cluster and cannot be changed until the migration completes")
	}
	return nil, nil
}

// controllerUser is the controller of this module: only it releases the lock, as only it knows whether the
// guest runs in the peer.
const controllerUser = "system:serviceaccount:d8-virtualization:virtualization-controller"

// validateLockOwner keeps the lock of a move to the controller and the cluster administrators: a VM released
// by hand starts here while the guest may run in the peer.
func validateLockOwner(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) error {
	for _, ann := range []string{annotations.AnnVMMovedToPeer, annotations.AnnVMMigrationReceiver} {
		if old := oldVM.GetAnnotations()[ann]; old == "" || newVM.GetAnnotations()[ann] == old {
			continue
		}
		req, err := admission.RequestFromContext(ctx)
		if err != nil {
			return err
		}
		if req.UserInfo.Username == controllerUser || slices.Contains(req.UserInfo.Groups, "system:masters") {
			return nil
		}
		return fmt.Errorf("the annotation %s is managed by the migration to a peer cluster. Delete the VirtualMachineOperation to cancel the migration", ann)
	}
	return nil
}
