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

package network

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// GetIPAddressTakingVMIPAddress need not return the mirror of the vmip: any Static
// IPAddress on the address counts. An Auto one counts only as a mirror, otherwise it
// belongs to a VM. Several may be on the address, and the sdn module gives it to one.
func GetIPAddressTakingVMIPAddress(ctx context.Context, c client.Client, vmip *v1alpha2.VirtualMachineIPAddress) (*sdnapi.IPAddress, error) {
	address := vmip.Status.Address
	if address == "" {
		return nil, nil
	}

	ipas := &sdnapi.IPAddressList{}
	if err := c.List(ctx, ipas, client.InNamespace(vmip.Namespace)); err != nil {
		return nil, fmt.Errorf("list SDN IPAddress in %s: %w", vmip.Namespace, err)
	}

	var ipasWithVMIPAddress []sdnapi.IPAddress
	for _, ipa := range ipas.Items {
		if !IsMainNetworkIPAddress(&ipa) {
			continue
		}
		static := ipa.Spec.Static != nil && ipa.Spec.Static.IP == address
		autoMirror := ipa.Spec.Static == nil && IsMirror(&ipa) && ipa.Status.Address == address
		if static || autoMirror {
			ipasWithVMIPAddress = append(ipasWithVMIPAddress, ipa)
		}
	}
	if len(ipasWithVMIPAddress) == 0 {
		return nil, nil
	}

	ipa := slices.MinFunc(ipasWithVMIPAddress, func(a, b sdnapi.IPAddress) int {
		return cmp.Or(
			compareFirst(a.GetDeletionTimestamp() == nil, b.GetDeletionTimestamp() == nil),
			compareFirst(IPAddressAllocated(&a), IPAddressAllocated(&b)),
			compareFirst(IsMirrorOf(&a, vmip), IsMirrorOf(&b, vmip)),
			a.GetCreationTimestamp().Compare(b.GetCreationTimestamp().Time),
			strings.Compare(a.GetName(), b.GetName()),
		)
	})
	return &ipa, nil
}

// GetMirrorOfVMIP returns one being deleted too, and nil when the name of the vmip
// belongs to another IPAddress.
func GetMirrorOfVMIP(ctx context.Context, c client.Reader, vmip *v1alpha2.VirtualMachineIPAddress) (*sdnapi.IPAddress, error) {
	ipa := &sdnapi.IPAddress{}
	if err := c.Get(ctx, types.NamespacedName{Name: vmip.Name, Namespace: vmip.Namespace}, ipa); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get the mirror IPAddress %q: %w", vmip.Name, err)
	}
	if !IsMirrorOf(ipa, vmip) {
		return nil, nil
	}
	return ipa, nil
}

func GetVMOfVMIP(ctx context.Context, c client.Client, vmip *v1alpha2.VirtualMachineIPAddress) (*v1alpha2.VirtualMachine, error) {
	vmName := vmip.Status.VirtualMachine
	for _, ref := range vmip.OwnerReferences {
		if ref.Kind == v1alpha2.VirtualMachineKind {
			vmName = ref.Name
			break
		}
	}
	if vmName == "" {
		return nil, nil
	}

	vm := &v1alpha2.VirtualMachine{}
	if err := c.Get(ctx, types.NamespacedName{Name: vmName, Namespace: vmip.Namespace}, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get VirtualMachine %q of vmip %q: %w", vmName, vmip.Name, err)
	}
	return vm, nil
}

// ReleaseTerminatingMirror unlinks a mirror being deleted that no pod runs on: the sdn
// module counts an owned IPAddress as used and never finishes deleting it.
func ReleaseTerminatingMirror(ctx context.Context, c client.Client, vmip *v1alpha2.VirtualMachineIPAddress, vm *v1alpha2.VirtualMachine) error {
	mirror, err := GetMirrorOfVMIP(ctx, c, vmip)
	if err != nil || mirror == nil || mirror.GetDeletionTimestamp() == nil {
		return err
	}
	if len(mirror.Status.UsedByPods) > 0 || (vm != nil && IPAddressInUse(vm, mirror)) {
		return nil
	}
	return unlinkMirror(ctx, c, mirror, true)
}

func unlinkMirror(ctx context.Context, c client.Client, ipa *sdnapi.IPAddress, dropOwnerRefs bool) error {
	_, hasVMLabel := ipa.GetLabels()[annotations.LabelVirtualMachineUID]
	hasOwnerRefsToDrop := dropOwnerRefs && len(ipa.GetOwnerReferences()) > 0
	if !hasVMLabel && !hasOwnerRefsToDrop {
		return nil
	}

	ipaWithoutVM := ipa.DeepCopy()
	labels := ipaWithoutVM.GetLabels()
	delete(labels, annotations.LabelVirtualMachineUID)
	ipaWithoutVM.SetLabels(labels)
	if hasOwnerRefsToDrop {
		ipaWithoutVM.SetOwnerReferences(nil)
	}
	if err := c.Patch(ctx, ipaWithoutVM, client.MergeFrom(ipa)); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("unlink the mirror IPAddress %q: %w", ipa.GetName(), err)
	}
	return nil
}

// MirrorOwnerRef: the sdn module counts an owned address as used, so the VM on the vmip
// owns the mirror. A VM that moved to an IPAddress of its own leaves the mirror to the
// vmip, so the address stays reserved until the VM names the vmip again.
func MirrorOwnerRef(vmip *v1alpha2.VirtualMachineIPAddress, vm *v1alpha2.VirtualMachine, vmMovedToIPAddress bool) *metav1.OwnerReference {
	switch {
	case vmMovedToIPAddress:
		return metav1.NewControllerRef(vmip, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineIPAddressKind))
	case vm != nil:
		return metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind))
	default:
		return nil
	}
}

// setMirrorOwner labels the mirror with the uid of a VM owner: the VM finds its
// IPAddress by the label. A mirror owned by the vmip gets no label: the VM has its own
// IPAddress, and two labelled ones would make the lookup ambiguous.
func setMirrorOwner(ipa *sdnapi.IPAddress, owner metav1.OwnerReference) {
	ipa.SetOwnerReferences([]metav1.OwnerReference{owner})
	if owner.Kind != v1alpha2.VirtualMachineKind {
		return
	}
	labels := ipa.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[annotations.LabelVirtualMachineUID] = string(owner.UID)
	ipa.SetLabels(labels)
}

func adoptMirror(ctx context.Context, c client.Client, ipa *sdnapi.IPAddress, owner *metav1.OwnerReference) error {
	if owner == nil || len(ipa.GetOwnerReferences()) > 0 {
		return nil
	}

	ipaWithOwner := ipa.DeepCopy()
	setMirrorOwner(ipaWithOwner, *owner)
	if err := c.Patch(ctx, ipaWithOwner, client.MergeFrom(ipa)); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsConflict(err) {
			return nil
		}
		return fmt.Errorf("adopt the mirror IPAddress %q: %w", ipa.GetName(), err)
	}
	return nil
}

func LabelMirrorWithVM(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, ipa *sdnapi.IPAddress) error {
	ipaWithVMLabel := ipa.DeepCopy()
	labels := ipaWithVMLabel.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[annotations.LabelVirtualMachineUID] = string(vm.GetUID())
	ipaWithVMLabel.SetLabels(labels)
	return c.Patch(ctx, ipaWithVMLabel, client.MergeFrom(ipa))
}

// ReleaseMirrorsOfPreviousVMIPs: with two labelled mirrors the lookup by label takes
// either. The ownerRef stays while a pod runs on the address, or the sdn module drops
// its route.
func ReleaseMirrorsOfPreviousVMIPs(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, vmip *v1alpha2.VirtualMachineIPAddress) error {
	ipas := &sdnapi.IPAddressList{}
	if err := c.List(ctx, ipas, client.InNamespace(vm.Namespace)); err != nil {
		return fmt.Errorf("list SDN IPAddress of the namespace %s: %w", vm.Namespace, err)
	}
	for i := range ipas.Items {
		ipa := &ipas.Items[i]
		if !isMirrorOfPreviousVMIP(ipa, vm, vmip) {
			continue
		}
		dropOwnerRefs := !IPAddressInUse(vm, ipa)
		if err := unlinkMirror(ctx, c, ipa, dropOwnerRefs); err != nil {
			return err
		}
	}
	return nil
}

// isMirrorOfPreviousVMIP checks the ownerRef too: the label is already gone from a
// mirror a pod still runs on.
func isMirrorOfPreviousVMIP(ipa *sdnapi.IPAddress, vm *v1alpha2.VirtualMachine, vmip *v1alpha2.VirtualMachineIPAddress) bool {
	if !IsMainNetworkIPAddress(ipa) || !IsMirror(ipa) || IsMirrorOf(ipa, vmip) {
		return false
	}
	if ipa.GetLabels()[annotations.LabelVirtualMachineUID] == string(vm.GetUID()) {
		return true
	}
	return slices.ContainsFunc(ipa.GetOwnerReferences(), func(ref metav1.OwnerReference) bool {
		return ref.UID == vm.GetUID()
	})
}

// EnsureVMIPAddressTaken returns any IPAddress taking the vmip address and creates a
// mirror only if none does. ErrIPAddressTerminating: a mirror created next to one being
// deleted would be refused the address. Nil: the name of the mirror is taken.
func EnsureVMIPAddressTaken(ctx context.Context, c client.Client, vmip *v1alpha2.VirtualMachineIPAddress, owner *metav1.OwnerReference) (*sdnapi.IPAddress, error) {
	ipa, err := GetIPAddressTakingVMIPAddress(ctx, c, vmip)
	switch {
	case err != nil:
		return nil, err
	case ipa == nil:
		return createMirror(ctx, c, vmip, MainStaticIPAddressSpec(vmip.Status.Address), owner)
	case ipa.GetDeletionTimestamp() != nil:
		return nil, ErrIPAddressTerminating
	case !IsMirrorOf(ipa, vmip):
		// Adopting a resource of somebody else would delete it together with the machine.
		return ipa, nil
	default:
		return ipa, adoptMirror(ctx, c, ipa, owner)
	}
}

// EnsureAutoMirror lets the sdn module pick the address of an Auto vmip, so the leases
// of deleted IPAddresses are respected. Nil: the name of the mirror is taken.
func EnsureAutoMirror(ctx context.Context, c client.Client, vmip *v1alpha2.VirtualMachineIPAddress, owner *metav1.OwnerReference) (*sdnapi.IPAddress, error) {
	existing, err := GetMirrorOfVMIP(ctx, c, vmip)
	switch {
	case err != nil:
		return nil, err
	case existing == nil:
		spec := sdnapi.IPAddressSpec{NetworkRef: SDNNetworkRef(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}), Type: sdnapi.IPAddressTypeAuto}
		return createMirror(ctx, c, vmip, spec, owner)
	case existing.GetDeletionTimestamp() != nil:
		return nil, ErrIPAddressTerminating
	default:
		return existing, nil
	}
}

// createMirror returns nil when the name belongs to another resource.
func createMirror(ctx context.Context, c client.Client, vmip *v1alpha2.VirtualMachineIPAddress, spec sdnapi.IPAddressSpec, owner *metav1.OwnerReference) (*sdnapi.IPAddress, error) {
	ipa := &sdnapi.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        vmip.Name,
			Namespace:   vmip.Namespace,
			Annotations: map[string]string{annotations.AnnMirrorOfVMIP: string(vmip.UID)},
		},
		Spec: spec,
	}
	if owner != nil {
		setMirrorOwner(ipa, *owner)
	}
	err := c.Create(ctx, ipa)
	switch {
	case err == nil:
		return ipa, nil
	case apierrors.IsAlreadyExists(err):
		existing, err := GetMirrorOfVMIP(ctx, c, vmip)
		if err == nil && existing != nil && existing.GetDeletionTimestamp() != nil {
			return nil, ErrIPAddressTerminating
		}
		return existing, err
	default:
		return nil, fmt.Errorf("create the mirror IPAddress of vmip %s: %w", vmip.Name, err)
	}
}

func IsMirror(ipa *sdnapi.IPAddress) bool {
	return ipa.GetAnnotations()[annotations.AnnMirrorOfVMIP] != ""
}

func IsMirrorOf(ipa *sdnapi.IPAddress, vmip *v1alpha2.VirtualMachineIPAddress) bool {
	vmipUID := ipa.GetAnnotations()[annotations.AnnMirrorOfVMIP]
	return vmipUID != "" && vmipUID == string(vmip.UID)
}

// VMIPAddressesTakenInSDNPool counts any allocated IPAddress on the address, not only
// the mirror.
func VMIPAddressesTakenInSDNPool(ctx context.Context, c client.Client) (bool, error) {
	vmips := &v1alpha2.VirtualMachineIPAddressList{}
	if err := c.List(ctx, vmips, client.UnsafeDisableDeepCopy); err != nil {
		return false, fmt.Errorf("list VirtualMachineIPAddress: %w", err)
	}
	ipas := &sdnapi.IPAddressList{}
	if err := c.List(ctx, ipas, client.UnsafeDisableDeepCopy); err != nil {
		return false, fmt.Errorf("list SDN IPAddress: %w", err)
	}
	taken := make(map[string]struct{}, len(ipas.Items))
	for i := range ipas.Items {
		ipa := &ipas.Items[i]
		if !IsMainNetworkIPAddress(ipa) {
			continue
		}
		if IPAddressAllocated(ipa) && ipa.Status.Address != "" {
			taken[ipa.Status.Address] = struct{}{}
		}
	}
	for i := range vmips.Items {
		address := vmips.Items[i].Status.Address
		if address == "" {
			continue
		}
		if _, ok := taken[address]; !ok {
			logger.FromContext(ctx).Debug("A vmip address is not taken in the sdn pool yet", "vmip", vmips.Items[i].Namespace+"/"+vmips.Items[i].Name, "address", address)
			return false, nil
		}
	}
	return true, nil
}
