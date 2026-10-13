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
	"encoding/json"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// MainClusterNetworkName is the PrimaryCNI ClusterNetwork of the Main network in SDN.
const MainClusterNetworkName = "main"

// MainIPAddressPoolName is the ClusterIPAddressPool of the Main network addresses;
// its ranges come from the virtualMachineCIDRs module setting.
const MainIPAddressPoolName = "d8-virtualization"

// MainIPAMAvailable: the PrimaryCNI ClusterNetwork "main" has an address pool. Read on
// every call, not cached: the modules can be enabled or updated at any time.
func MainIPAMAvailable(ctx context.Context, c client.Reader) (bool, error) {
	// The chart enables the SDN gate together with the sdn module. Without it the
	// kind is not registered, and a lookup of it costs a discovery request.
	if !featuregates.Default().Enabled(featuregates.SDN) {
		return false, nil
	}

	clusterNetwork := &sdnapi.ClusterNetwork{}
	if err := c.Get(ctx, types.NamespacedName{Name: MainClusterNetworkName}, clusterNetwork); err != nil {
		if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return false, nil
		}
		return false, fmt.Errorf("get ClusterNetwork %s: %w", MainClusterNetworkName, err)
	}
	if clusterNetwork.Spec.Type != sdnapi.ClusterNetworkTypePrimaryCNI {
		return false, nil
	}
	ipam := clusterNetwork.Spec.IPAM
	return ipam != nil && ipam.IPAddressPoolRef != nil && ipam.IPAddressPoolRef.Name != "", nil
}

// MainInterfaceSpecRequired: the pod cannot start without the Main interface spec. A VM
// on an IPAddress has no other source of the address, and once the template has the
// spec the sdn module routes the address by it.
func MainInterfaceSpecRequired(usesVMIP bool, kvvm *virtv1.VirtualMachine) bool {
	return !usesVMIP || KVVMHasMainInterfaceSpec(kvvm)
}

func MainInterfaceSpecEnabled(ctx context.Context, c client.Reader, usesVMIP bool, kvvm *virtv1.VirtualMachine) (bool, error) {
	if MainInterfaceSpecRequired(usesVMIP, kvvm) {
		return true, nil
	}
	return MainIPAMAvailable(ctx, c)
}

func KVVMHasMainInterfaceSpec(kvvm *virtv1.VirtualMachine) bool {
	return kvvm != nil && kvvm.Spec.Template != nil && HasMainInterfaceSpec(kvvm.Spec.Template.ObjectMeta.Annotations)
}

func HasMainInterfaceSpec(anno map[string]string) bool {
	var specs InterfaceSpecList
	if err := json.Unmarshal([]byte(anno[annotations.AnnNetworksSpec]), &specs); err != nil {
		return false
	}
	return specs.MainWithAddress() != nil
}

// vmHasOwnVMIP looks only for the vmip the controller created: a vmip named in the
// spec is the owner's.
func vmHasOwnVMIP(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) (bool, error) {
	list := &v1alpha2.VirtualMachineIPAddressList{}
	err := c.List(ctx, list, client.InNamespace(vm.GetNamespace()),
		client.MatchingLabels{annotations.LabelVirtualMachineUID: string(vm.GetUID())})
	if err != nil {
		return false, fmt.Errorf("list VirtualMachineIPAddress of the virtual machine: %w", err)
	}
	for i := range list.Items {
		if list.Items[i].DeletionTimestamp == nil {
			return true, nil
		}
	}
	return false, nil
}

// vmHasMainIPAddress counts a terminating IPAddress too: it is recreated on the same address.
func vmHasMainIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) (bool, error) {
	mainSpec := MainNetworkSpecOrImplicit(vm)
	if mainSpec == nil {
		return false, nil
	}
	ipa, err := GetSDNIPAddressOfVM(ctx, c, vm, *mainSpec)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return false, nil
		}
		return false, err
	}
	return ipa != nil, nil
}

// UsesVMIP reports whether the Main network address of the machine comes
// from a VirtualMachineIPAddress rather than from an IPAddress. The first that holds
// decides:
//   - a vmip named in the spec: the vmip;
//   - an IPAddress named in the spec: the IPAddress, while the sdn module serves the
//     Main network or the template has the Main interface spec;
//   - the machine keeps the vmip or the IPAddress it already has;
//   - a new machine gets an IPAddress where the sdn module serves the Main network
//     and has allocated the address of every vmip: until then it could give the new
//     machine the address of a vmip.
func UsesVMIP(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) (bool, error) {
	if vm.Spec.VirtualMachineIPAddress != "" {
		return true, nil
	}
	mainIPAMAvailable, err := MainIPAMAvailable(ctx, c)
	if err != nil {
		return false, err
	}

	if mainSpec := GetMainNetworkSpec(vm.Spec.Networks); mainSpec != nil && mainSpec.IPAddressName != "" {
		// Once the template has the Main interface spec the named IPAddress stays in
		// effect without the pool: the sdn module keeps routing the address by that spec.
		namedIPAddressInEffect := mainIPAMAvailable
		if !namedIPAddressInEffect {
			namedIPAddressInEffect, err = templateHasMainInterfaceSpec(ctx, c, vm)
			if err != nil {
				return false, err
			}
		}
		if namedIPAddressInEffect {
			return false, nil
		}
	}

	hasVMIP, err := vmHasOwnVMIP(ctx, c, vm)
	if err != nil || hasVMIP {
		return hasVMIP, err
	}
	hasIPAddress, err := vmHasMainIPAddress(ctx, c, vm)
	if err != nil || hasIPAddress {
		return false, err
	}
	if !mainIPAMAvailable {
		return true, nil
	}
	// Until every vmip address is taken in the sdn pool, sdn could give one to a new machine.
	vmipAddressesTaken, err := VMIPAddressesTakenInSDNPool(ctx, c)
	if err != nil {
		return false, err
	}
	return !vmipAddressesTaken, nil
}

func templateHasMainInterfaceSpec(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) (bool, error) {
	kvvm := &virtv1.VirtualMachine{}
	if err := c.Get(ctx, types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, kvvm); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	return KVVMHasMainInterfaceSpec(kvvm), nil
}

// MainNetworkSpecOrImplicit returns nil when the VM has additional networks only.
func MainNetworkSpecOrImplicit(vm *v1alpha2.VirtualMachine) *v1alpha2.NetworksSpec {
	return GetMainNetworkSpec(WithImplicitMain(vm.Spec.Networks))
}

var ErrIPAddressOfAnotherNetwork = errors.New("the IPAddress belongs to another network")

// IPAddressConflictError is a user configuration state, not a failure: it is
// reported in a condition, not retried.
type IPAddressConflictError struct {
	VMName string
}

func (e *IPAddressConflictError) Error() string {
	return fmt.Sprintf("the IPAddress is referenced by the virtual machine %q too", e.VMName)
}

// GetMainSDNIPAddress returns the named IPAddress or the one the VM got by its label,
// a terminating one too: whether the VM may use it is for CheckMainSDNIPAddress to say.
func GetMainSDNIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) (*sdnapi.IPAddress, error) {
	mainSpec := MainNetworkSpecOrImplicit(vm)
	if mainSpec == nil {
		return nil, nil
	}
	if mainSpec.IPAddressName != "" {
		return GetSDNIPAddress(ctx, c, vm.Namespace, mainSpec.IPAddressName)
	}
	return GetSDNIPAddressOfVM(ctx, c, vm, *mainSpec)
}

// ErrIPAddressBeingDeleted means the IPAddress is being deleted and no pod of the VM
// runs on it any more.
var ErrIPAddressBeingDeleted = errors.New("the IPAddress is being deleted")

var ErrIPAddressNotAllocated = errors.New("the IPAddress has no address yet")

// CheckMainSDNIPAddress returns why the VM cannot put the IPAddress of its Main
// network into its pod, or nil.
func CheckMainSDNIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, ipa *sdnapi.IPAddress) error {
	if mainSpec := MainNetworkSpecOrImplicit(vm); mainSpec != nil && mainSpec.IPAddressName != "" {
		if err := checkNamedMainIPAddress(ctx, c, vm, ipa, *mainSpec); err != nil {
			return err
		}
	}
	if !IPAddressServesVM(vm, ipa) {
		return ErrIPAddressBeingDeleted
	}
	if !IPAddressAllocated(ipa) || ipa.Status.Address == "" {
		return ErrIPAddressNotAllocated
	}
	return nil
}

// checkNamedMainIPAddress: the IPAddress the VM got by its label is its own and always fits.
func checkNamedMainIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, ipa *sdnapi.IPAddress, mainSpec v1alpha2.NetworksSpec) error {
	if !IsMainNetworkIPAddress(ipa) {
		return ErrIPAddressOfAnotherNetwork
	}
	conflictVM, err := OtherVMUsingIPAddress(ctx, c, vm, ipa, mainSpec)
	if err != nil {
		return err
	}
	// An IPAddress referenced by two machines stays with the one that already has its
	// address in status.ipAddress, running or stopped; the other one is blocked. Until
	// the IPAddress has an address, both are.
	if conflictVM != "" && (ipa.Status.Address == "" || ipa.Status.Address != vm.Status.IPAddress) {
		return &IPAddressConflictError{VMName: conflictVM}
	}
	return nil
}

// IsIPAddressConfigurationError reports whether err from CheckMainSDNIPAddress is
// about the configuration of the VM, which only its owner can fix.
func IsIPAddressConfigurationError(err error) bool {
	var conflict *IPAddressConflictError
	return errors.Is(err, ErrIPAddressOfAnotherNetwork) || errors.As(err, &conflict)
}

// IsIPAddressUnusable reports whether err from CheckMainSDNIPAddress is a reason the
// VM cannot use the IPAddress rather than a failure to check it.
func IsIPAddressUnusable(err error) bool {
	return IsIPAddressConfigurationError(err) || errors.Is(err, ErrIPAddressBeingDeleted) || errors.Is(err, ErrIPAddressNotAllocated)
}

// usableMainIPAddress: the spec carries the address, not only the name, so cni-cilium
// does not read the IPAddress from the API server while a pod is being created.
func usableMainIPAddress(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine) (*sdnapi.IPAddress, error) {
	ipa, err := GetMainSDNIPAddress(ctx, c, vm)
	if err != nil || ipa == nil {
		return nil, err
	}
	if err := CheckMainSDNIPAddress(ctx, c, vm, ipa); err != nil {
		// The IPAMHandler reports the reason to the user.
		if IsIPAddressUnusable(err) {
			return nil, nil
		}
		return nil, err
	}
	return ipa, nil
}
