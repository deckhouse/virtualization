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
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// IPAddressGVK is the GroupVersionKind of the SDN IPAddress resource
// (network.deckhouse.io/v1alpha1). IPAddress is namespaced and used for
// additional network interfaces of pods/VMs (both Network and ClusterNetwork).
var IPAddressGVK = schema.GroupVersionKind{Group: "network.deckhouse.io", Version: "v1alpha1", Kind: "IPAddress"}

// SDN IPAddress condition types and reasons (mirrors network.deckhouse.io/v1alpha1).
const (
	sdnConditionTypeAllocated = "Allocated"

	sdnReasonNoFreeIPAddress = "NoFreeIPAddress"
)

// SDN IPAddress phases.
const (
	SDNIPAddressPhasePending   = "Pending"
	SDNIPAddressPhaseAllocated = "Allocated"
)

// SDNIPAddressTypeAuto is the Auto allocation type for SDN IPAddress.
const SDNIPAddressTypeAuto = "Auto"

// EnsureSDNIPAddress creates an SDN IPAddress (type Auto) for the given VM and
// additional network, with ownerReferences on the VirtualMachine and a label
// linking it to the VM (analogous to vmip/vmmac). It is idempotent: if an
// IPAddress owned by this VM for this network already exists, it is returned
// as-is. The IPAddress is namespaced (in the VM namespace).
//
// Returns the name of the IPAddress (existing or newly created) and an error.
// Transient API errors are returned as-is for the caller to requeue.
func EnsureSDNIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, netSpec v1alpha2.NetworksSpec) (string, error) {
	if vm == nil || netSpec.Type == v1alpha2.NetworksTypeMain {
		return "", nil
	}

	// Try to find an existing IPAddress owned by this VM for this network.
	existing, err := FindSDNIPAddress(ctx, c, vm, netSpec)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}

	// Create a new IPAddress (Auto) owned by the VM.
	gvk := v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineKind)
	ownerRef := metav1.NewControllerRef(vm, gvk)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(IPAddressGVK)
	obj.SetGenerateName(vm.GetName() + "-")
	obj.SetNamespace(vm.Namespace)
	obj.SetLabels(map[string]string{
		annotations.LabelVirtualMachineUID: string(vm.GetUID()),
	})
	obj.SetOwnerReferences([]metav1.OwnerReference{*ownerRef})

	// spec.networkRef
	if err := unstructured.SetNestedField(obj.Object, map[string]any{
		"kind": netSpec.Type,
		"name": netSpec.Name,
	}, "spec", "networkRef"); err != nil {
		return "", fmt.Errorf("set spec.networkRef: %w", err)
	}
	// spec.type = Auto
	if err := unstructured.SetNestedField(obj.Object, SDNIPAddressTypeAuto, "spec", "type"); err != nil {
		return "", fmt.Errorf("set spec.type: %w", err)
	}

	if err := c.Create(ctx, obj); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race: another controller instance created it. Re-find.
			return FindSDNIPAddress(ctx, c, vm, netSpec)
		}
		return "", fmt.Errorf("create SDN IPAddress for %s: %w", SpecKey(netSpec), err)
	}
	return obj.GetName(), nil
}

// FindSDNIPAddress returns the name of the SDN IPAddress owned by the given VM
// for the given additional network, or "" if not found. It lists IPAddress
// resources in the VM namespace filtered by the VM UID label and matches by
// spec.networkRef (kind+name).
func FindSDNIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, netSpec v1alpha2.NetworksSpec) (string, error) {
	if vm == nil || netSpec.Type == v1alpha2.NetworksTypeMain {
		return "", nil
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   IPAddressGVK.Group,
		Version: IPAddressGVK.Version,
		Kind:    "IPAddressList",
	})

	labelSelector := client.MatchingLabels{
		annotations.LabelVirtualMachineUID: string(vm.GetUID()),
	}
	if err := c.List(ctx, list, client.InNamespace(vm.Namespace), labelSelector); err != nil {
		return "", fmt.Errorf("list SDN IPAddress for %s: %w", SpecKey(netSpec), err)
	}

	for _, item := range list.Items {
		kind, _, _ := unstructured.NestedString(item.Object, "spec", "networkRef", "kind")
		name, _, _ := unstructured.NestedString(item.Object, "spec", "networkRef", "name")
		if kind == netSpec.Type && name == netSpec.Name {
			return item.GetName(), nil
		}
	}
	return "", nil
}

// DeleteSDNIPAddress deletes the SDN IPAddress owned by the given VM for the
// given network (if any). Used when the network is removed from the VM spec.
func DeleteSDNIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, netSpec v1alpha2.NetworksSpec) error {
	if vm == nil || netSpec.Type == v1alpha2.NetworksTypeMain {
		return nil
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   IPAddressGVK.Group,
		Version: IPAddressGVK.Version,
		Kind:    "IPAddressList",
	})

	labelSelector := client.MatchingLabels{
		annotations.LabelVirtualMachineUID: string(vm.GetUID()),
	}
	if err := c.List(ctx, list, client.InNamespace(vm.Namespace), labelSelector); err != nil {
		return fmt.Errorf("list SDN IPAddress for deletion %s: %w", SpecKey(netSpec), err)
	}

	for _, item := range list.Items {
		kind, _, _ := unstructured.NestedString(item.Object, "spec", "networkRef", "kind")
		name, _, _ := unstructured.NestedString(item.Object, "spec", "networkRef", "name")
		if kind == netSpec.Type && name == netSpec.Name {
			if err := c.Delete(ctx, &item); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete SDN IPAddress %s: %w", item.GetName(), err)
			}
		}
	}
	return nil
}

// SDNIPAddressInfo holds identifying info of an SDN IPAddress owned by a VM.
type SDNIPAddressInfo struct {
	Name           string
	NetworkRefKind string
	NetworkRefName string
}

// ListSDNIPAddressesForVM lists all SDN IPAddress resources owned by the given
// VM (by label virtual-machine-uid). Returns identifying info for each.
func ListSDNIPAddressesForVM(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) ([]SDNIPAddressInfo, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   IPAddressGVK.Group,
		Version: IPAddressGVK.Version,
		Kind:    "IPAddressList",
	})
	labelSelector := client.MatchingLabels{
		annotations.LabelVirtualMachineUID: string(vm.GetUID()),
	}
	if err := c.List(ctx, list, client.InNamespace(vm.Namespace), labelSelector); err != nil {
		return nil, fmt.Errorf("list SDN IPAddress for VM %s: %w", vm.Name, err)
	}
	result := make([]SDNIPAddressInfo, 0, len(list.Items))
	for _, item := range list.Items {
		kind, _, _ := unstructured.NestedString(item.Object, "spec", "networkRef", "kind")
		name, _, _ := unstructured.NestedString(item.Object, "spec", "networkRef", "name")
		result = append(result, SDNIPAddressInfo{
			Name:           item.GetName(),
			NetworkRefKind: kind,
			NetworkRefName: name,
		})
	}
	return result, nil
}

// DeleteSDNIPAddressByName deletes an SDN IPAddress by name in the given namespace.
func DeleteSDNIPAddressByName(ctx context.Context, c client.Client, namespace, name string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(IPAddressGVK)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete SDN IPAddress %s/%s: %w", namespace, name, err)
	}
	return nil
}

// SDNIPAddressStatus holds the relevant status fields of an SDN IPAddress.
type SDNIPAddressStatus struct {
	Name    string
	Address string
	Phase   string
	// NoFreeAddress is true when the pool is exhausted (condition Allocated=False, reason=NoFreeIPAddress).
	NoFreeAddress bool
	// Allocated is true when the address has been allocated (condition Allocated=True).
	Allocated bool
}

// GetSDNIPAddressStatus fetches the SDN IPAddress by name and returns its
// status. Returns nil (without error) if the IPAddress does not exist.
func GetSDNIPAddressStatus(ctx context.Context, c client.Client, namespace, name string) (*SDNIPAddressStatus, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(IPAddressGVK)
	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get SDN IPAddress %s/%s: %w", namespace, name, err)
	}

	status := &SDNIPAddressStatus{Name: name}
	status.Address, _, _ = unstructured.NestedString(obj.Object, "status", "address")
	status.Phase, _, _ = unstructured.NestedString(obj.Object, "status", "phase")

	conds, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found {
		for _, c := range conds {
			m, ok := c.(map[string]any)
			if !ok {
				continue
			}
			typ, _, _ := unstructured.NestedString(m, "type")
			if typ != sdnConditionTypeAllocated {
				continue
			}
			condStatus, _, _ := unstructured.NestedString(m, "status")
			reason, _, _ := unstructured.NestedString(m, "reason")
			if condStatus == string(metav1.ConditionTrue) {
				status.Allocated = true
			}
			if reason == sdnReasonNoFreeIPAddress {
				status.NoFreeAddress = true
			}
			break
		}
	}
	return status, nil
}

// SDNIPAddressExists checks whether a static (user-provided) SDN IPAddress
// with the given name exists and is bound to the given network. Used for
// static-mode validation.
func SDNIPAddressExists(ctx context.Context, c client.Client, namespace, name, networkKind, networkName string) (bool, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(IPAddressGVK)
	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get SDN IPAddress %s/%s: %w", namespace, name, err)
	}
	kind, _, _ := unstructured.NestedString(obj.Object, "spec", "networkRef", "kind")
	refName, _, _ := unstructured.NestedString(obj.Object, "spec", "networkRef", "name")
	return kind == networkKind && refName == networkName, nil
}

// IsIPAddressNameUsedByAnotherVM checks if any other VM in the same namespace
// references the given ipAddressName in spec.networks[].ipAddressName for the
// given network (same type + name). This prevents two VMs from using the same
// static IPAddress simultaneously.
//
// Returns the name of the conflicting VM (if any) and nil error.
func IsIPAddressNameUsedByAnotherVM(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, ipAddressName string, netSpec v1alpha2.NetworksSpec) (string, error) {
	var vms v1alpha2.VirtualMachineList
	if err := c.List(ctx, &vms, client.InNamespace(vm.Namespace)); err != nil {
		return "", fmt.Errorf("list VMs in namespace %s: %w", vm.Namespace, err)
	}
	for _, other := range vms.Items {
		if other.Name == vm.Name {
			continue
		}
		for _, net := range other.Spec.Networks {
			if net.IPAddressName == ipAddressName && net.Type == netSpec.Type && net.Name == netSpec.Name {
				return other.Name, nil
			}
		}
	}
	return "", nil
}
