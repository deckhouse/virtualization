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

package vm

import (
	"context"
	"errors"
	"fmt"
	"slices"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

// PeerMigrationSnapshotError rejects a peer migration while a snapshot or filesystem freeze is active.
func PeerMigrationSnapshotError(vm *v1alpha2.VirtualMachine) error {
	if meta.IsStatusConditionTrue(vm.Status.Conditions, vmcondition.TypeSnapshotting.String()) {
		return fmt.Errorf("VirtualMachine %q cannot be migrated to a peer cluster while a snapshot is pending or in progress: wait for the snapshot to finish and retry the migration", vm.Name)
	}

	if meta.IsStatusConditionTrue(vm.Status.Conditions, vmcondition.TypeFilesystemFrozen.String()) {
		return fmt.Errorf("VirtualMachine %q cannot be migrated to a peer cluster while its filesystem is frozen: wait for the filesystem to be unfrozen and retry the migration", vm.Name)
	}

	return nil
}

// PeerMigrationLockError returns an error if migration to or from a peer cluster locks the VM.
func PeerMigrationLockError(ctx context.Context, c client.Client, v *v1alpha2.VirtualMachine) error {
	if v == nil || c == nil {
		return nil
	}
	// The mark on the KubeVirt VM comes first, the condition a moment later; the condition also stands for
	// the mark while the KubeVirt VM is not seen.
	op := ""

	var kvvm virtv1.VirtualMachine
	if err := c.Get(ctx, types.NamespacedName{Name: v.Name, Namespace: v.Namespace}, &kvvm); err == nil {
		op = PeerMigrationVMOP(&kvvm)
	}

	if op != "" || meta.IsStatusConditionTrue(v.Status.Conditions, vmcondition.TypeMovedToPeer.String()) {
		var vmop v1alpha2.VirtualMachineOperation
		if op != "" {
			if err := c.Get(ctx, types.NamespacedName{Name: op, Namespace: v.Namespace}, &vmop); err == nil && commonvmop.PeerName(&vmop) != "" {
				return fmt.Errorf("the virtual machine is being migrated to peer cluster %q. Cancel the migration by deleting the VirtualMachineOperation first", commonvmop.PeerName(&vmop))
			}
		}

		return errors.New("the virtual machine is being migrated to a peer cluster. Cancel the migration by deleting the VirtualMachineOperation first")
	}

	received := slices.ContainsFunc(v.GetOwnerReferences(), func(ref metav1.OwnerReference) bool {
		return ref.Kind == intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiverKind
	})
	if received || meta.IsStatusConditionTrue(v.Status.Conditions, vmcondition.TypeReceivingFromPeer.String()) {
		return errors.New("the virtual machine is being migrated from a peer cluster and cannot be changed until the migration completes")
	}

	return nil
}

// PeerMigrationLockErrorForDisk returns an error if any VirtualMachine that references the named
// VirtualDisk is locked by a move to or from a peer cluster, or nil when no such machine is locked.
// A nil client is allowed and returns nil.
func PeerMigrationLockErrorForDisk(ctx context.Context, c client.Client, diskName, namespace string) error {
	if c == nil {
		return nil
	}

	var vms v1alpha2.VirtualMachineList
	if err := c.List(ctx, &vms, client.InNamespace(namespace)); err != nil {
		return err
	}

	for i := range vms.Items {
		v := &vms.Items[i]

		lockErr := PeerMigrationLockError(ctx, c, v)
		if lockErr == nil {
			continue
		}

		references := referencesDisk(v, diskName)
		if !references {
			var kvvm virtv1.VirtualMachine
			if err := c.Get(ctx, types.NamespacedName{Name: v.Name, Namespace: v.Namespace}, &kvvm); err != nil {
				if !k8serrors.IsNotFound(err) {
					return err
				}
			} else {
				spec, err := LoadLastAppliedSpec(&kvvm)
				if err != nil {
					return err
				}

				if spec != nil {
					references = referencesDiskSpec(spec, diskName)
				}
			}
		}

		if !references {
			continue
		}

		return lockErr
	}

	return nil
}

func referencesDisk(v *v1alpha2.VirtualMachine, diskName string) bool {
	if referencesDiskSpec(&v.Spec, diskName) {
		return true
	}

	for _, ref := range v.Status.BlockDeviceRefs {
		if ref.Kind == v1alpha2.DiskDevice && ref.Name == diskName && ref.Attached {
			return true
		}
	}

	return false
}

func referencesDiskSpec(spec *v1alpha2.VirtualMachineSpec, diskName string) bool {
	for _, ref := range spec.BlockDeviceRefs {
		if ref.Kind == v1alpha2.DiskDevice && ref.Name == diskName {
			return true
		}
	}

	return false
}

// PeerMigrationVMOP returns the name of the operation that moves the VM of the KubeVirt VM to a peer
// cluster, or nothing when it does not move.
func PeerMigrationVMOP(kvvm *virtv1.VirtualMachine) string {
	if kvvm == nil {
		return ""
	}

	return kvvm.GetAnnotations()[annotations.AnnPeerMigrationVMOP]
}
