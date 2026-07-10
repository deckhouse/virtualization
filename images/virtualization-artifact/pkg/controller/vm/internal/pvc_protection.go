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

package internal

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// vmRequiresPVCProtection reports whether block device volumes of the virtual
// machine may be mounted on a node, so the backing PVCs must not be released.
// This holds from the moment the runtime objects start being created until
// they are completely torn down: deleting a PVC earlier lets the storage
// backend destroy the data under a live mount (e.g. an NFS-backed hotplug
// volume bind-mounted into the virt-launcher pod turns into a stale file
// handle that can never be unmounted through the safe path).
func vmRequiresPVCProtection(vm *v1alpha2.VirtualMachine) bool {
	if vm == nil {
		return false
	}
	if !vm.GetDeletionTimestamp().IsZero() {
		return true
	}
	switch vm.Status.Phase {
	case v1alpha2.MachineStarting,
		v1alpha2.MachineRunning,
		v1alpha2.MachineStopping,
		v1alpha2.MachineTerminating,
		v1alpha2.MachineMigrating,
		v1alpha2.MachinePause,
		v1alpha2.MachineDegraded:
		return true
	}
	return false
}

// volumeClaimNames collects the names of PVCs referenced by KVVM and KVVMI
// volumes into claims. KVVMI volumeStatus is inspected as well: a hotplugged
// volume stays there until virt-handler confirms it is unmounted from the
// virt-launcher pod, even after the volume is removed from both specs.
func volumeClaimNames(claims map[string]struct{}, kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) {
	addVolumes := func(volumes []virtv1.Volume) {
		for _, volume := range volumes {
			switch {
			case volume.PersistentVolumeClaim != nil:
				claims[volume.PersistentVolumeClaim.ClaimName] = struct{}{}
			case volume.MemoryDump != nil:
				claims[volume.MemoryDump.ClaimName] = struct{}{}
			}
		}
	}

	if kvvm != nil && kvvm.Spec.Template != nil {
		addVolumes(kvvm.Spec.Template.Spec.Volumes)
	}

	if kvvmi != nil {
		addVolumes(kvvmi.Spec.Volumes)
		for _, vs := range kvvmi.Status.VolumeStatus {
			if vs.PersistentVolumeClaimInfo != nil && vs.PersistentVolumeClaimInfo.ClaimName != "" {
				claims[vs.PersistentVolumeClaimInfo.ClaimName] = struct{}{}
			}
		}
	}
}

// protectedClaimNames returns the names of PVCs in the namespace that back
// volumes of virtual machines still requiring protection.
func protectedClaimNames(ctx context.Context, cl client.Client, namespace string) (map[string]struct{}, error) {
	var vms v1alpha2.VirtualMachineList
	if err := cl.List(ctx, &vms, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachines: %w", err)
	}

	claims := make(map[string]struct{})

	for i := range vms.Items {
		vm := &vms.Items[i]
		if !vmRequiresPVCProtection(vm) {
			continue
		}

		key := types.NamespacedName{Name: vm.GetName(), Namespace: vm.GetNamespace()}

		kvvm, err := object.FetchObject(ctx, key, cl, &virtv1.VirtualMachine{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch KVVM %q: %w", key, err)
		}

		kvvmi, err := object.FetchObject(ctx, key, cl, &virtv1.VirtualMachineInstance{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch KVVMI %q: %w", key, err)
		}

		volumeClaimNames(claims, kvvm, kvvmi)
	}

	return claims, nil
}

// reconcilePVCProtection ensures the pvc-protection finalizer is set on PVCs
// backing volumes of virtual machines that require protection and is removed
// from all other PVCs in the namespace. Working namespace-wide keeps shared
// (multi-attached) PVCs protected while at least one virtual machine needs
// them and releases PVCs whose volumes are already gone from the specs of a
// stopped virtual machine.
func reconcilePVCProtection(ctx context.Context, cl client.Client, protection *service.ProtectionService, namespace string) error {
	protectedClaims, err := protectedClaimNames(ctx, cl, namespace)
	if err != nil {
		return err
	}

	var pvcs corev1.PersistentVolumeClaimList
	if err := cl.List(ctx, &pvcs, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list PersistentVolumeClaims: %w", err)
	}

	for i := range pvcs.Items {
		pvc := &pvcs.Items[i]
		if _, ok := protectedClaims[pvc.GetName()]; ok {
			if err := protection.AddProtection(ctx, pvc); err != nil {
				return err
			}
		} else if controllerutil.ContainsFinalizer(pvc, v1alpha2.FinalizerPVCProtection) {
			if err := protection.RemoveProtection(ctx, pvc); err != nil {
				return err
			}
		}
	}

	return nil
}
