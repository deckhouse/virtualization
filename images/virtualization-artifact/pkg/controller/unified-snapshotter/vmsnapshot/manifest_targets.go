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

package vmsnapshot

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// errManifestTargetNotReady signals that a resource referenced by the VirtualMachine (its VMIP, a VMMAC,
// the provisioner Secret, or a VMBDA) does not exist
var errManifestTargetNotReady = errors.New("a resource referenced for manifest capture does not exist")

func manifestTargetNotReady(kind, name string) error {
	return fmt.Errorf("%w: %s %q", errManifestTargetNotReady, kind, name)
}

// planManifestTargets builds the full manifest-capture target set for vm, mirroring the resource list the
// old (non-SDK) mechanism stores in its snapshot secret (see SecretRestorer.Store in
// virtualization-artifact's pkg/controller/service/restorer/secret_restorer.go): the VirtualMachine
// itself, its VirtualMachineIPAddress (subject to vms.Spec.KeepIPAddress), one VirtualMachineMACAddress
// per secondary network, the provisioner Secret (if any), and one VirtualMachineBlockDeviceAttachment per
// hotplugged block device.
//
// The full set must be known before the first call to EnsureManifestCapture: the underlying
// ManifestCaptureRequest's Targets are immutable once created, so partial/growing target sets across
// reconciles are not an option — see the package-level doc comment.
func (r *Reconciler) planManifestTargets(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine) ([]snapshotsdk.ManifestTarget, error) {
	targets := []snapshotsdk.ManifestTarget{{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualMachineKind,
		Name:       vm.Name,
	}}

	vmipTarget, err := r.planVMIPTarget(ctx, vms, vm)
	if err != nil {
		return nil, err
	}
	if vmipTarget != nil {
		targets = append(targets, *vmipTarget)
	}

	macTargets, err := r.planVMMACTargets(ctx, vm)
	if err != nil {
		return nil, err
	}
	targets = append(targets, macTargets...)

	secretTarget, err := r.planProvisionerSecretTarget(ctx, vm)
	if err != nil {
		return nil, err
	}
	if secretTarget != nil {
		targets = append(targets, *secretTarget)
	}

	vmbdaTargets, err := r.planVMBDATargets(ctx, vm)
	if err != nil {
		return nil, err
	}
	targets = append(targets, vmbdaTargets...)

	return targets, nil
}

// planVMIPTarget returns the VirtualMachineIPAddress target, or nil if the old mechanism's rules exclude
// it: with KeepIPAddress: Never, an anonymous (unnamed) Auto-type IP is not worth preserving across
// restore, since a fresh one would be allocated anyway.
func (r *Reconciler) planVMIPTarget(ctx context.Context, vms *v1alpha2.VirtualMachineSnapshot, vm *v1alpha2.VirtualMachine) (*snapshotsdk.ManifestTarget, error) {
	name := vm.Status.VirtualMachineIPAddress
	if name == "" {
		return nil, nil
	}

	vmip := &v1alpha2.VirtualMachineIPAddress{}
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: name}, vmip); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, manifestTargetNotReady(v1alpha2.VirtualMachineIPAddressKind, name)
		}
		return nil, err
	}

	if vms.Spec.KeepIPAddress == v1alpha2.KeepIPAddressNever {
		named := vmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeAuto && vm.Spec.VirtualMachineIPAddress != ""
		if vmip.Spec.Type != v1alpha2.VirtualMachineIPAddressTypeStatic && !named {
			return nil, nil
		}
	}

	return &snapshotsdk.ManifestTarget{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualMachineIPAddressKind,
		Name:       vmip.Name,
	}, nil
}

// planVMMACTargets returns one target per secondary-network VirtualMachineMACAddress (the pod/main
// network's MAC is not user/restore-relevant and is skipped, matching the old mechanism).
func (r *Reconciler) planVMMACTargets(ctx context.Context, vm *v1alpha2.VirtualMachine) ([]snapshotsdk.ManifestTarget, error) {
	var targets []snapshotsdk.ManifestTarget
	for _, n := range vm.Status.Networks {
		if n.Type == v1alpha2.NetworksTypeMain || n.VirtualMachineMACAddressName == "" {
			continue
		}

		vmmac := &v1alpha2.VirtualMachineMACAddress{}
		if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: n.VirtualMachineMACAddressName}, vmmac); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, manifestTargetNotReady(v1alpha2.VirtualMachineMACAddressKind, n.VirtualMachineMACAddressName)
			}
			return nil, err
		}

		targets = append(targets, snapshotsdk.ManifestTarget{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineMACAddressKind,
			Name:       vmmac.Name,
		})
	}
	return targets, nil
}

// planProvisionerSecretTarget returns the Secret backing vm.Spec.Provisioning's userDataRef/sysprepRef, or
// nil if the VM has no provisioning configured (or it's inline UserData, which has no Secret to capture).
func (r *Reconciler) planProvisionerSecretTarget(ctx context.Context, vm *v1alpha2.VirtualMachine) (*snapshotsdk.ManifestTarget, error) {
	p := vm.Spec.Provisioning
	if p == nil {
		return nil, nil
	}

	var secretName string
	switch p.Type {
	case v1alpha2.ProvisioningTypeUserDataRef:
		if p.UserDataRef == nil || p.UserDataRef.Kind != v1alpha2.UserDataRefKindSecret {
			return nil, fmt.Errorf("virtual machine %q: provisioning userDataRef must reference a Secret", vm.Name)
		}
		secretName = p.UserDataRef.Name
	case v1alpha2.ProvisioningTypeSysprepRef:
		if p.SysprepRef == nil || p.SysprepRef.Kind != v1alpha2.SysprepRefKindSecret {
			return nil, fmt.Errorf("virtual machine %q: provisioning sysprepRef must reference a Secret", vm.Name)
		}
		secretName = p.SysprepRef.Name
	default:
		// ProvisioningTypeUserData: inline data, no backing Secret to capture.
		return nil, nil
	}

	secret := &corev1.Secret{}
	if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: secretName}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, manifestTargetNotReady("Secret", secretName)
		}
		return nil, err
	}

	return &snapshotsdk.ManifestTarget{
		APIVersion: "v1",
		Kind:       "Secret",
		Name:       secret.Name,
	}, nil
}

// planVMBDATargets returns one target per hotplugged block device that has a backing
// VirtualMachineBlockDeviceAttachment object (block devices attached at VM-creation time have none, and
// are captured as part of the VirtualMachine manifest itself).
func (r *Reconciler) planVMBDATargets(ctx context.Context, vm *v1alpha2.VirtualMachine) ([]snapshotsdk.ManifestTarget, error) {
	var targets []snapshotsdk.ManifestTarget
	for _, bdr := range vm.Status.BlockDeviceRefs {
		if !bdr.Hotplugged || bdr.VirtualMachineBlockDeviceAttachmentName == "" {
			continue
		}

		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{}
		if err := r.APIReader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: bdr.VirtualMachineBlockDeviceAttachmentName}, vmbda); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, manifestTargetNotReady(v1alpha2.VirtualMachineBlockDeviceAttachmentKind, bdr.VirtualMachineBlockDeviceAttachmentName)
			}
			return nil, err
		}

		targets = append(targets, snapshotsdk.ManifestTarget{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineBlockDeviceAttachmentKind,
			Name:       vmbda.Name,
		})
	}
	return targets, nil
}
