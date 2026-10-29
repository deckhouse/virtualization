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

package vmop

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/peering"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// peerMigrationValidator refuses a migration to a peer cluster that cannot succeed, before anything is
// created in the peer. It needs nothing from the peer: what the peer checks itself, it reports through
// the request to receive.
type peerMigrationValidator struct {
	client client.Client
	peers  pairedPeers
}

// pairedPeers tells whether a project is paired with a peer that offers virtualization to it.
type pairedPeers interface {
	Paired(ctx context.Context, peer, namespace string) error
}

func (v *peerMigrationValidator) ValidateCreate(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (admission.Warnings, error) {
	if err := checkPeerMigrationType(vmop); err != nil {
		return nil, err
	}
	if commonvmop.PeerName(vmop) == "" {
		return nil, nil
	}
	if !featuregates.Default().Enabled(featuregates.Peering) {
		return nil, errors.New("migration to a peer cluster requires the Peering feature gate")
	}
	if err := v.peers.Paired(ctx, commonvmop.PeerName(vmop), vmop.Namespace); err != nil {
		if errors.Is(err, peering.ErrPeerNotPaired) {
			return nil, fmt.Errorf("the cluster %q is not paired with this project or does not support virtual machine migration", commonvmop.PeerName(vmop))
		}
		return nil, fmt.Errorf("failed to check the peer cluster: %w", err)
	}

	vm := &v1alpha2.VirtualMachine{}
	if err := v.client.Get(ctx, types.NamespacedName{Name: vmop.Spec.VirtualMachine, Namespace: vmop.Namespace}, vm); err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine: %w", err)
	}
	return nil, checkPeerMigratable(vm)
}

// checkPeerMigrationType refuses projectPeerName on an operation that does not move the VM to a peer: the
// in-cluster lifecycle would silently run it here instead. The CRD schema already forbids spec.migrate
// outside Migrate; the check keeps an old schema from turning a move into a local migration.
func checkPeerMigrationType(vmop *v1alpha2.VirtualMachineOperation) error {
	if commonvmop.PeerName(vmop) != "" && vmop.Spec.Type != v1alpha2.VMOPTypeMigrate {
		return errors.New("projectPeerName can only be set for the Migrate operation")
	}
	return nil
}

// checkPeerMigratable tells why a virtual machine cannot move to a peer cluster: the peer receives only a
// running virtual machine with its VirtualDisks, without devices of the node and hot-plugged disks.
func checkPeerMigratable(vm *v1alpha2.VirtualMachine) error {
	if vm.Status.Phase != v1alpha2.MachineRunning {
		return fmt.Errorf("only a running virtual machine can move to a peer cluster, %q is %s", vm.Name, vm.Status.Phase)
	}
	for _, ref := range vm.OwnerReferences {
		if ref.Kind == v1alpha2.VirtualMachinePoolKind {
			return fmt.Errorf("the virtual machine %q is a replica of VirtualMachinePool %q: replicas do not move to a peer cluster", vm.Name, ref.Name)
		}
	}
	if len(vm.Spec.USBDevices) > 0 || len(vm.Spec.PCIDevices) > 0 {
		return fmt.Errorf("the virtual machine %q uses USB or PCI devices of its node: it cannot move to a peer cluster", vm.Name)
	}
	for _, bd := range vm.Status.BlockDeviceRefs {
		// Every disk is hot-plugged now; a VirtualMachineBlockDeviceAttachment is what does not move.
		if bd.VirtualMachineBlockDeviceAttachmentName != "" {
			return fmt.Errorf("the disk %q is attached with VirtualMachineBlockDeviceAttachment: detach it before moving to a peer cluster", bd.Name)
		}
		if bd.Kind != v1alpha2.DiskDevice {
			return fmt.Errorf("the %s %q cannot move to a peer cluster: only VirtualDisks move", bd.Kind, bd.Name)
		}
	}
	return nil
}

// lockedVMValidator refuses operations on a VM that moves to a peer cluster or is being received from
// one: the move owns the VM until it ends, and the other operations would race with it.
type lockedVMValidator struct {
	client client.Client
}

func (v *lockedVMValidator) ValidateCreate(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (admission.Warnings, error) {
	vm := &v1alpha2.VirtualMachine{}
	if err := v.client.Get(ctx, types.NamespacedName{Name: vmop.Spec.VirtualMachine, Namespace: vmop.Namespace}, vm); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return nil, checkVMLocked(ctx, v.client, vm)
}

// checkVMLocked tells why a VM cannot be changed now: it moves to a peer cluster or arrives from one.
func checkVMLocked(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) error {
	if op := vm.GetAnnotations()[annotations.AnnVMMovedToPeer]; op != "" {
		var vmop v1alpha2.VirtualMachineOperation
		if err := c.Get(ctx, types.NamespacedName{Name: op, Namespace: vm.Namespace}, &vmop); err == nil && commonvmop.PeerName(&vmop) != "" {
			return fmt.Errorf("the virtual machine is being migrated to cluster %q. Cancel the migration by deleting the VirtualMachineOperation first", commonvmop.PeerName(&vmop))
		}
		return errors.New("the virtual machine is being migrated to a peer cluster. Cancel the migration by deleting the VirtualMachineOperation first")
	}
	if vm.GetAnnotations()[annotations.AnnVMMigrationReceiver] != "" {
		return errors.New("the virtual machine is being migrated from a peer cluster and cannot be changed until the migration completes")
	}
	return nil
}
