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
	"errors"
	"fmt"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func SDNNetworkRef(netSpec v1alpha2.NetworksSpec) sdnapi.NetworkRef {
	if netSpec.Type == v1alpha2.NetworksTypeMain {
		return sdnapi.NetworkRef{Kind: v1alpha2.NetworksTypeClusterNetwork, Name: MainClusterNetworkName}
	}
	return sdnapi.NetworkRef{Kind: netSpec.Type, Name: netSpec.Name}
}

func IPAddressBelongsToNetwork(ipa *sdnapi.IPAddress, netSpec v1alpha2.NetworksSpec) bool {
	ref := SDNNetworkRef(netSpec)
	return ipa.Spec.NetworkRef.Kind == ref.Kind && ipa.Spec.NetworkRef.Name == ref.Name
}

// EnsureSDNIPAddress picks the IPAddress of the network among ipAddresses (those of
// the VM) or creates an Auto one.
func EnsureSDNIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, netSpec v1alpha2.NetworksSpec, ipAddresses []sdnapi.IPAddress) (string, error) {
	if existing := PreferredSDNIPAddress(vm, ipAddressesOfNetwork(ipAddresses, netSpec)); existing != nil {
		return liveIPAddressName(existing)
	}
	return createSDNIPAddress(ctx, c, vm, sdnapi.IPAddressSpec{NetworkRef: SDNNetworkRef(netSpec), Type: sdnapi.IPAddressTypeAuto})
}

// EnsureMainSDNIPAddress is EnsureSDNIPAddress for the Main network, where the
// address must not change: a replacement for a resource deleted by hand is Static
// with the address to keep (the one the machine holds, or the one of the source for
// a machine received from another cluster), not a new Auto one. An address another
// IPAddress holds is not kept: a Static one would stay Pending.
func EnsureMainSDNIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, address string, ipAddresses []sdnapi.IPAddress) (string, error) {
	mainSpec := v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}
	if existing := PreferredSDNIPAddress(vm, ipAddressesOfNetwork(ipAddresses, mainSpec)); existing != nil {
		return liveIPAddressName(existing)
	}

	spec := sdnapi.IPAddressSpec{NetworkRef: SDNNetworkRef(mainSpec), Type: sdnapi.IPAddressTypeAuto}
	if address != "" {
		ipas, err := MainSDNIPAddressesByAddress(ctx, c)
		if err != nil {
			return "", err
		}
		if _, ok := ipas[address]; !ok {
			spec = MainStaticIPAddressSpec(address)
		}
	}
	return createSDNIPAddress(ctx, c, vm, spec)
}

func liveIPAddressName(ipa *sdnapi.IPAddress) (string, error) {
	if ipa.GetDeletionTimestamp() != nil {
		return "", ErrIPAddressTerminating
	}
	return ipa.GetName(), nil
}

// MainSDNIPAddressesByAddress counts an IPAddress being deleted too: its address is
// free only once the sdn module is done with it.
func MainSDNIPAddressesByAddress(ctx context.Context, c client.Client) (map[string]types.NamespacedName, error) {
	list := &sdnapi.IPAddressList{}
	if err := c.List(ctx, list, client.UnsafeDisableDeepCopy); err != nil {
		return nil, fmt.Errorf("list SDN IPAddress: %w", err)
	}
	result := make(map[string]types.NamespacedName)
	for i := range list.Items {
		ipa := &list.Items[i]
		if !IsMainNetworkIPAddress(ipa) {
			continue
		}
		if address := AddressOfIPAddress(ipa); address != "" {
			result[address] = types.NamespacedName{Namespace: ipa.GetNamespace(), Name: ipa.GetName()}
		}
	}
	return result, nil
}

func AddressOfIPAddress(ipa *sdnapi.IPAddress) string {
	if ipa.Spec.Static != nil && ipa.Spec.Static.IP != "" {
		return ipa.Spec.Static.IP
	}
	return ipa.Status.Address
}

// The uid label is how ListSDNIPAddressesOfVM finds the IPAddress of the VM.
func createSDNIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, spec sdnapi.IPAddressSpec) (string, error) {
	ipa := &sdnapi.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: vm.GetName() + "-",
			Namespace:    vm.Namespace,
			Labels: map[string]string{
				annotations.LabelVirtualMachineUID: string(vm.GetUID()),
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(vm, v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind)),
			},
		},
		Spec: spec,
	}
	if err := c.Create(ctx, ipa); err != nil {
		return "", fmt.Errorf("create SDN IPAddress of the network %s/%s: %w", spec.NetworkRef.Kind, spec.NetworkRef.Name, err)
	}
	return ipa.GetName(), nil
}

// ReleaseTerminatingSDNIPAddresses drops the ownerRef of the terminating IPAddresses
// of the machine that no pod of it may run on: sdn counts an owned IPAddress as used
// and never finishes deleting it. The label stays, so the machine waits until it is gone.
func ReleaseTerminatingSDNIPAddresses(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, ipAddresses []sdnapi.IPAddress) error {
	for i := range ipAddresses {
		ipa := &ipAddresses[i]
		if ipa.GetDeletionTimestamp() == nil || len(ipa.GetOwnerReferences()) == 0 || IPAddressInUse(vm, ipa) {
			continue
		}
		patched := ipa.DeepCopy()
		patched.SetOwnerReferences(nil)
		if err := c.Patch(ctx, patched, client.MergeFrom(ipa)); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("release the terminating IPAddress %q: %w", ipa.GetName(), err)
		}
	}
	return nil
}

func ListSDNIPAddressesOfVM(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) ([]sdnapi.IPAddress, error) {
	list := &sdnapi.IPAddressList{}
	labelSelector := client.MatchingLabels{
		annotations.LabelVirtualMachineUID: string(vm.GetUID()),
	}
	if err := c.List(ctx, list, client.InNamespace(vm.Namespace), labelSelector); err != nil {
		return nil, fmt.Errorf("list SDN IPAddress of the virtual machine %s: %w", vm.Name, err)
	}
	return list.Items, nil
}

func ipAddressesOfNetwork(ipAddresses []sdnapi.IPAddress, netSpec v1alpha2.NetworksSpec) []sdnapi.IPAddress {
	var result []sdnapi.IPAddress
	for i := range ipAddresses {
		if IPAddressBelongsToNetwork(&ipAddresses[i], netSpec) {
			result = append(result, ipAddresses[i])
		}
	}
	return result
}

// GetSDNIPAddressOfVM returns one being deleted too, and nil if there is none.
func GetSDNIPAddressOfVM(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, netSpec v1alpha2.NetworksSpec) (*sdnapi.IPAddress, error) {
	items, err := ListSDNIPAddressesOfVM(ctx, c, vm)
	if err != nil {
		return nil, err
	}
	return PreferredSDNIPAddress(vm, ipAddressesOfNetwork(items, netSpec)), nil
}

// PreferredSDNIPAddress picks the IPAddress the VM uses among those of one network,
// or returns nil if there are none. There can be several: the name is generated,
// so a reconcile on a stale cache creates a second one, and a VM moving from its own
// IPAddress to a vmip has both that and the mirror. A live one goes first, then the
// mirror (only the mirror of the current vmip carries the label of the VM), then the
// one in use, then the oldest.
func PreferredSDNIPAddress(vm *v1alpha2.VirtualMachine, items []sdnapi.IPAddress) *sdnapi.IPAddress {
	if len(items) == 0 {
		return nil
	}
	preferred := slices.MinFunc(items, func(a, b sdnapi.IPAddress) int {
		return cmp.Or(
			compareFirst(a.GetDeletionTimestamp() == nil, b.GetDeletionTimestamp() == nil),
			compareFirst(IsMirror(&a), IsMirror(&b)),
			compareFirst(IPAddressInUse(vm, &a), IPAddressInUse(vm, &b)),
			a.GetCreationTimestamp().Compare(b.GetCreationTimestamp().Time),
			strings.Compare(a.GetName(), b.GetName()),
		)
	})
	return &preferred
}

// compareFirst orders a true condition before a false one.
func compareFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

// IPAddressInUse reports whether a pod of the VM may still run on the IPAddress, so
// it must not be released or deleted. Either sign is enough: sdn lists in usedByPods
// only the pods that took the address from the Main interface spec, and a pod that
// took it from cni.cilium.io/ipAddress is seen only as the address of the running VM.
func IPAddressInUse(vm *v1alpha2.VirtualMachine, ipa *sdnapi.IPAddress) bool {
	return len(ipa.Status.UsedByPods) > 0 || vmRunsOnIPAddress(vm, ipa)
}

// IPAddressServesVM reports whether the VM may still put the IPAddress into its pod.
// One being deleted does only while the VM runs on it and sdn still routes it to a
// pod. Both signs are needed, unlike in IPAddressInUse: a new pod given an IPAddress
// being deleted would lose its address once sdn drops the resource.
func IPAddressServesVM(vm *v1alpha2.VirtualMachine, ipa *sdnapi.IPAddress) bool {
	return ipa.GetDeletionTimestamp() == nil || (len(ipa.Status.UsedByPods) > 0 && vmRunsOnIPAddress(vm, ipa))
}

// vmRunsOnIPAddress reports whether a live pod of the VM serves the address of the
// Main network IPAddress. A starting pod is not counted: it waits for the
// replacement of a terminating IPAddress.
func vmRunsOnIPAddress(vm *v1alpha2.VirtualMachine, ipa *sdnapi.IPAddress) bool {
	switch vm.Status.Phase {
	case v1alpha2.MachineRunning, v1alpha2.MachineMigrating, v1alpha2.MachinePause:
	default:
		return false
	}
	return IsMainNetworkIPAddress(ipa) && ipa.Status.Address != "" && ipa.Status.Address == vm.Status.IPAddress
}

// ErrMainIPAddressNotReady means the pod needs the Main interface spec, and the VM
// has no IPAddress it can put there yet: the desired internal virtual machine
// cannot be built until it has. The IPAddressReady condition says why.
var ErrMainIPAddressNotReady = errors.New("the IPAddress of the Main network is not ready yet")

// ErrIPAddressTerminating means the address of this VM and network is still being
// deleted. The name is generated, so creating a replacement every reconcile while
// it lives would leave the VM with as many addresses as the deletion is slow.
var ErrIPAddressTerminating = errors.New("the IPAddress of the virtual machine is terminating")

func DeleteSDNIPAddressByName(ctx context.Context, c client.Client, namespace, name string) error {
	ipa := &sdnapi.IPAddress{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if err := c.Delete(ctx, ipa); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete SDN IPAddress %s/%s: %w", namespace, name, err)
	}
	return nil
}

// GetSDNIPAddress returns nil, not an error, when the IPAddress does not exist: the
// callers tell a missing one from one that holds no address yet.
func GetSDNIPAddress(ctx context.Context, c client.Client, namespace, name string) (*sdnapi.IPAddress, error) {
	ipa := &sdnapi.IPAddress{}
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, ipa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get SDN IPAddress %s/%s: %w", namespace, name, err)
	}
	return ipa, nil
}

// MainStaticIPAddressSpec returns the spec of an IPAddress of the Main network that
// keeps the address. The lease of a deleted resource on the address is ignored: it
// outlives the resource by the pool TTL.
func MainStaticIPAddressSpec(address string) sdnapi.IPAddressSpec {
	return sdnapi.IPAddressSpec{
		NetworkRef: SDNNetworkRef(v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}),
		Type:       sdnapi.IPAddressTypeStatic,
		Static:     &sdnapi.StaticIPAddress{IP: address, IgnoreLeaseTTLOnAllocation: true},
	}
}

// MainIPAddressToSnapshot returns an empty name for a VM on a vmip: the snapshot keeps
// the vmip, and the IPAddress in the status is its mirror.
func MainIPAddressToSnapshot(vm *v1alpha2.VirtualMachine) (name, address string) {
	if vm.Status.VirtualMachineIPAddress != "" || MainNetworkSpec(vm.Spec.Networks) == nil {
		return "", ""
	}
	for _, status := range vm.Status.Networks {
		if status.Type != v1alpha2.NetworksTypeMain || status.IPAddress == "" {
			continue
		}
		return status.IPAddressName, status.IPAddress
	}
	return "", ""
}

// With KeepIPAddress: Never a snapshot keeps only such an IPAddress, as it keeps only
// a Static vmip.
func IsStaticOrNamedMainIPAddress(vm *v1alpha2.VirtualMachine, ipa *sdnapi.IPAddress) bool {
	if ipa.Spec.Type == sdnapi.IPAddressTypeStatic {
		return true
	}
	mainSpec := MainNetworkSpec(vm.Spec.Networks)
	return mainSpec != nil && mainSpec.IPAddressName == ipa.GetName()
}

// DetachIPAddressFromVM drops what ties the IPAddress to the snapshotted VM: the
// ownerRef, or GC would delete it with that VM; the VM label and the mirror
// annotation, or the controllers would take it for the IPAddress of that VM or for
// the mirror of its vmip.
func DetachIPAddressFromVM(ipa *sdnapi.IPAddress) {
	ipa.SetOwnerReferences(nil)
	labels := ipa.GetLabels()
	delete(labels, annotations.LabelVirtualMachineUID)
	ipa.SetLabels(labels)
	anns := ipa.GetAnnotations()
	delete(anns, annotations.AnnMirrorOfVMIP)
	ipa.SetAnnotations(anns)
}

// Empty networks mean the implicit Main network, so it is added when the name is set.
// Networks without the Main network are returned as they are.
func SetMainIPAddressName(networks []v1alpha2.NetworksSpec, name string) []v1alpha2.NetworksSpec {
	for i := range networks {
		if networks[i].Type == v1alpha2.NetworksTypeMain {
			networks[i].IPAddressName = name
			return networks
		}
	}
	if name != "" && UsesMainNetwork(networks) {
		return []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: name}}
	}
	return networks
}

func IsMainNetworkIPAddress(ipa *sdnapi.IPAddress) bool {
	return IPAddressBelongsToNetwork(ipa, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain})
}

func IPAddressAllocated(ipa *sdnapi.IPAddress) bool {
	return ipa != nil && apimeta.IsStatusConditionTrue(ipa.Status.Conditions, sdnapi.ConditionAllocated)
}

func IPAddressTakenByAnother(ipa *sdnapi.IPAddress) bool {
	return allocationFailedWithReason(ipa, sdnapi.ReasonIPAddressIsUsed)
}

func IPAddressPoolExhausted(ipa *sdnapi.IPAddress) bool {
	return allocationFailedWithReason(ipa, sdnapi.ReasonNoFreeIPAddress)
}

func allocationFailedWithReason(ipa *sdnapi.IPAddress, reason string) bool {
	if ipa == nil {
		return false
	}
	cond := apimeta.FindStatusCondition(ipa.Status.Conditions, sdnapi.ConditionAllocated)
	return cond != nil && cond.Status != metav1.ConditionTrue && cond.Reason == reason
}

// AdditionalNetworkIPAddress returns the IPAddress the pod gets on an additional
// network with a pool, or nil while there is none it can use: the one named in the
// spec must belong to the network and to this VM alone, and either must be allocated.
func AdditionalNetworkIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, netSpec v1alpha2.NetworksSpec) (*sdnapi.IPAddress, error) {
	if netSpec.IPAddressName == "" {
		ipa, err := GetSDNIPAddressOfVM(ctx, c, vm, netSpec)
		if err != nil || ipa == nil || ipa.GetDeletionTimestamp() != nil || !IPAddressAllocated(ipa) {
			return nil, err
		}
		return ipa, nil
	}

	ipa, err := GetSDNIPAddress(ctx, c, vm.Namespace, netSpec.IPAddressName)
	if err != nil || ipa == nil || !IPAddressBelongsToNetwork(ipa, netSpec) || !IPAddressAllocated(ipa) {
		return nil, err
	}
	conflictVM, err := OtherVMUsingIPAddress(ctx, c, vm, ipa, netSpec)
	if err != nil || conflictVM != "" {
		return nil, err
	}
	return ipa, nil
}

// OtherVMUsingIPAddress returns the name of another VM of the namespace that uses
// the IPAddress for the same network, by its spec or as the owner, or "".
func OtherVMUsingIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, ipa *sdnapi.IPAddress, netSpec v1alpha2.NetworksSpec) (string, error) {
	var vms v1alpha2.VirtualMachineList
	if err := c.List(ctx, &vms, client.InNamespace(vm.Namespace)); err != nil {
		return "", fmt.Errorf("list VMs in namespace %s: %w", vm.Namespace, err)
	}
	vmNameByUID := make(map[string]string, len(vms.Items))
	for _, other := range vms.Items {
		if other.Name == vm.Name {
			continue
		}
		vmNameByUID[string(other.UID)] = other.Name
		for _, net := range other.Spec.Networks {
			if net.IPAddressName == ipa.GetName() && net.Type == netSpec.Type && net.Name == netSpec.Name {
				return other.Name, nil
			}
		}
	}

	// An IPAddress the VM gets without ipAddressName is not in its spec, but its name
	// is in status.networks and can be copied into a second VM.
	owner := ipa.GetLabels()[annotations.LabelVirtualMachineUID]
	if owner == "" || owner == string(vm.GetUID()) {
		return "", nil
	}
	// An owner that no longer exists leaves a stale label on a resource the
	// orphan collector is about to delete: that is not a conflict.
	return vmNameByUID[owner], nil
}
