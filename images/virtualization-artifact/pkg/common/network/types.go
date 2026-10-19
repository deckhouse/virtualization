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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const NameDefaultInterface = "default"

// IPAssignmentModeDHCP is the ipAssignmentMode value indicating that SDN should
// raise a DHCP server on the tap interface to deliver the allocated IP address
// into the guest OS.
const IPAssignmentModeDHCP = "DHCP"

// BindingModeVFIOPCI is the networks-spec bindingMode for SR-IOV VF passthrough:
// SDN binds the VF to vfio-pci and delivers it via its DRA driver.
const BindingModeVFIOPCI = "VFIO-PCI"

// SDNResourceClaimTemplateName returns the name of the ResourceClaimTemplate the
// SDN module maintains for the given UnderlayNetwork in every namespace labeled
// direct-nic-access.network.deckhouse.io/enabled.
func SDNResourceClaimTemplateName(underlayNetworkName string) string {
	return "d8-sdn-" + underlayNetworkName
}

// UnderlayDeviceName returns the name used for both the pod-level resource claim
// and the KubeVirt host device of an UnderlayNetwork entry. The "un-" prefix keeps
// the claims apart from USB/GPU-owned ones, whose garbage collection matches by
// name prefix.
func UnderlayDeviceName(networkName string) string {
	return "un-" + networkName
}

// WillProvisionInterface reports whether the given additional network interface
// will be included in networks-spec (i.e. SDN will provision it and report
// status). This mirrors the skip logic in ProvisionableInterfaceSpecs:
//   - network without a pool: provisioned as L2-only (unless ipAddressName is
//     set, which is a config error -> skip);
//   - network with a pool, static mode: provisioned only if the IPAddress exists;
//   - network with a pool, auto mode: provisioned only if the auto IPAddress
//     exists and is allocated.
//
// Used by NetworkInterfaceHandler to avoid waiting for SDN status on skipped
// interfaces (which would produce a misleading "waiting for SDN" message).
func WillProvisionInterface(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, netSpec v1alpha2.NetworksSpec) (bool, error) {
	if netSpec.Type == v1alpha2.NetworksTypeMain {
		return false, nil
	}
	if netSpec.Type == v1alpha2.NetworksTypeUnderlayNetwork {
		// UnderlayNetwork entries are always provisioned by SDN (no IPAM involved).
		return true, nil
	}
	hasPool, err := HasIPAM(ctx, c, vm.Namespace, netSpec)
	if err != nil {
		return false, err
	}
	if !hasPool {
		// No pool: L2-only is provisioned unless ipAddressName is set (config error).
		return netSpec.IPAddressName == "", nil
	}
	ipa, err := AdditionalNetworkIPAddress(ctx, c, vm, netSpec)
	return ipa != nil, err
}

// ProvisionableInterfaceSpecs returns the interface specs the pod can get now, with
// the IPAddress names of the additional networks filled in; an interface that cannot
// be provisioned yet is left out. A network with a pool has spec.ipam.ipAddressPoolRef:
//   - auto-mode (no user ipAddressName): finds the SDN IPAddress created by the
//     IPAddressHandler (by VM label) and includes its name. If the IPAddress is
//     not yet allocated (Pending/NoFreeIPAddress) or not found, the interface is
//     skipped (not included in the result) so CNI does not fail.
//   - static-mode (user ipAddressName): validates the referenced IPAddress
//     exists. If not found, the interface is skipped.
//   - networks without a pool: kept as L2-only (no ipAssignmentMode).
//
// The Main network is always kept in the result (KubeVirt needs its interface).
// Where the Main interface spec is enabled (see MainInterfaceSpecEnabled), it carries
// the name and the address of the usable Main IPAddress, but no ipAssignmentMode:
// cni-cilium applies the address on the pod interface. Otherwise the Main interface
// spec carries no address and stays out of the annotation.
//
// This must be applied everywhere the networks-spec annotation is built so that
// the annotation is consistent between the KVVM template (initial pod creation)
// and a direct pod patch (live reconfiguration).
func ProvisionableInterfaceSpecs(ctx context.Context, c client.Client, vm *v1alpha2.VirtualMachine, specs InterfaceSpecList, mainInterfaceSpecEnabled bool) (InterfaceSpecList, error) {
	result := make(InterfaceSpecList, 0, len(specs))
	for _, spec := range specs {
		if spec.Type == v1alpha2.NetworksTypeMain {
			// The Main interface spec without an address stays out of the annotation (see
			// ForAnnotation), and the pod takes the vmip address from cni.cilium.io/ipAddress.
			spec.IPAddressNames, spec.IPAddress = nil, ""
			if mainInterfaceSpecEnabled {
				ipa, err := usableMainIPAddress(ctx, c, vm)
				if err != nil {
					// A transient failure must not silently drop the Main interface spec
					// from the annotation of a running pod.
					return nil, fmt.Errorf("get the Main IPAddress: %w", err)
				}
				if ipa != nil {
					spec.IPAddressNames, spec.IPAddress = []string{ipa.GetName()}, ipa.Status.Address
				}
			}
			result = append(result, spec)
			continue
		}
		hasPool, err := HasIPAM(ctx, c, vm.Namespace, v1alpha2.NetworksSpec{
			Type: spec.Type,
			Name: spec.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("check IPAM for network %s: %w", spec.Name, err)
		}
		if !hasPool {
			// No pool: IPAM is not enabled.
			if len(spec.IPAddressNames) > 0 {
				// User set ipAddressName on a network without a pool — this is a
				// configuration error. Skip the interface (do not include it in
				// networks-spec) so CNI does not fail trying to find the IPAddress.
				// The error is surfaced in NetworkReady by collectIPAMErrors.
				continue
			}
			// L2-only: no IPAM, keep as-is (without ipAddressNames).
			spec.IPAddressNames = nil
			result = append(result, spec)
			continue
		}

		// IPAM is enabled: resolve ipAddressNames and check readiness.
		netSpec := v1alpha2.NetworksSpec{Type: spec.Type, Name: spec.Name}
		if len(spec.IPAddressNames) > 0 {
			netSpec.IPAddressName = spec.IPAddressNames[0]
		}
		ipa, err := AdditionalNetworkIPAddress(ctx, c, vm, netSpec)
		if err != nil {
			return nil, fmt.Errorf("get the IPAddress of %s: %w", SpecKey(netSpec), err)
		}
		if ipa == nil {
			continue
		}
		spec.IPAddressNames = []string{ipa.GetName()}
		spec.IPAssignmentMode = IPAssignmentModeDHCP
		result = append(result, spec)
	}
	return result, nil
}

func HasMainNetworkStatus(networks []v1alpha2.NetworksStatus) bool {
	for _, network := range networks {
		if network.Type == v1alpha2.NetworksTypeMain {
			return true
		}
	}

	return false
}

type InterfaceSpec struct {
	ID            int    `json:"id"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	InterfaceName string `json:"ifName"`
	MAC           string `json:"-"`
	UID           int    `json:"uid"`
	GID           int    `json:"gid"`
	// IPAssignmentMode is the IPAM mode for the additional interface.
	// Set to "DHCP" when the referenced network has a pool, so SDN raises a DHCP
	// server on the tap interface to deliver the allocated IP into the guest OS.
	IPAssignmentMode string `json:"ipAssignmentMode,omitempty"`
	// IPAddressNames references existing IPAddress resources (SDN) to use for a
	// static IP on the interface instead of automatic allocation.
	// At most one name per interface (single-element slice per the SDN annotation format).
	IPAddressNames []string `json:"ipAddressNames,omitempty"`
	// IPAddress is set for the Main interface only: cni-cilium applies it as-is.
	// The sdn agent finds the addresses of additional networks by ipAddressNames.
	IPAddress string `json:"ipAddress,omitempty"`
	// BindingMode is how SDN hands the device to the pod. Set to "VFIO-PCI" for
	// UnderlayNetwork entries (SR-IOV VF passthrough); empty for tap-based entries.
	BindingMode string `json:"bindingMode,omitempty"`
	// VLANID is the VLAN tag SDN programs on the SR-IOV VF (UnderlayNetwork entries only).
	VLANID *int `json:"vlanID,omitempty"`
	// VFMAC is the MAC address SDN programs on the SR-IOV VF (UnderlayNetwork
	// entries only). Tap-based entries keep MAC out of the annotation (json:"-" above),
	// so this is a separate field to avoid changing their wire format.
	VFMAC string `json:"mac,omitempty"`
}

type InterfaceStatus struct {
	Type       string             `json:"type"`
	Name       string             `json:"name"`
	IfName     string             `json:"ifName"`
	Mac        string             `json:"mac"`
	Conditions []metav1.Condition `json:"conditions"`
	// IPAddressConfigs holds the IP addresses allocated by SDN for the interface.
	IPAddressConfigs []IPAddressConfig `json:"ipAddressConfigs,omitempty"`
}

// IPAddressConfig represents an IP address allocated by SDN for an interface.
// Mirrors the ipAddressConfigs entry of the network.deckhouse.io/networks-status annotation.
// Only Address is consumed by the VM controller to populate status.networks[].ipAddress.
// Address returns the allocated IP address (without mask).
// Network is the CIDR of the pool the address was allocated from.
// Routes are the routes applied to the address.
// Name is the name of the IPAddress resource.
// Remaining fields are preserved for future use.
type IPAddressConfig struct {
	Name    string    `json:"name,omitempty"`
	Address string    `json:"address,omitempty"`
	Network string    `json:"network,omitempty"`
	Routes  []IPRoute `json:"routes,omitempty"`
}

// IPRoute represents a route applied to an allocated IP address.
// Mirrors the routes entry of ipAddressConfigs in the networks-status annotation.
// Destination is the destination CIDR.
// Via is the gateway for the destination.
type IPRoute struct {
	Destination string `json:"destination,omitempty"`
	Via         string `json:"via,omitempty"`
}

type InterfaceSpecList []InterfaceSpec

// MainPodInterfaceName is the pod-side name of the primary (Main) interface in
// the networks-spec annotation. InterfaceName of the Main spec holds the
// KubeVirt network name ("default") and is not meaningful on the pod side.
const MainPodInterfaceName = "eth0"

// HasAddress reports whether the spec names an IPAddress and its address.
func (s InterfaceSpec) HasAddress() bool {
	return len(s.IPAddressNames) > 0 && s.IPAddress != ""
}

// HasAdditional reports whether the list has a network other than Main.
func (s InterfaceSpecList) HasAdditional() bool {
	for _, spec := range s {
		if spec.Type != v1alpha2.NetworksTypeMain {
			return true
		}
	}
	return false
}

// MainWithAddress returns the Main interface spec when it has an address, or nil.
func (s InterfaceSpecList) MainWithAddress() *InterfaceSpec {
	for i := range s {
		if s[i].Type == v1alpha2.NetworksTypeMain && s[i].HasAddress() {
			return &s[i]
		}
	}
	return nil
}

// ForAnnotation returns the interface specs that go into the networks-spec
// annotation. The Main interface spec goes only with an address: without
// ipAddressNames cni-cilium would create an IPAddress owned by the pod, and the
// address would die with it.
func (s InterfaceSpecList) ForAnnotation() InterfaceSpecList {
	filtered := InterfaceSpecList{}
	for _, spec := range s {
		if spec.Type == v1alpha2.NetworksTypeMain {
			if !spec.HasAddress() {
				continue
			}
			spec.InterfaceName = MainPodInterfaceName
		}
		filtered = append(filtered, spec)
	}
	return filtered
}

// BuildNetworksAnnotations returns the pod network annotations of the list: the
// key/values to set and the keys to remove. A list with nothing to advertise
// produces no annotations; the tap-provision annotation is advertised only when
// there is an SDN-provisioned (non-Main) interface.
func (s InterfaceSpecList) BuildNetworksAnnotations() (annotationsToSet map[string]string, annotationsToRemove []string, err error) {
	advertised := s.ForAnnotation()
	if len(advertised) == 0 {
		return nil, []string{annotations.AnnNetworksSpec, annotations.AnnTapProvisionByDVPSupported}, nil
	}
	data, err := json.Marshal(advertised)
	if err != nil {
		return nil, nil, err
	}
	annotationsToSet = map[string]string{annotations.AnnNetworksSpec: string(data)}
	if advertised.HasAdditional() {
		annotationsToSet[annotations.AnnTapProvisionByDVPSupported] = "true"
	} else {
		annotationsToRemove = []string{annotations.AnnTapProvisionByDVPSupported}
	}
	return annotationsToSet, annotationsToRemove, nil
}

func (s InterfaceSpecList) ToString() (string, error) {
	data, err := json.Marshal(s.ForAnnotation())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s InterfaceSpecList) HasMain() bool {
	for _, spec := range s {
		if spec.Type == v1alpha2.NetworksTypeMain {
			return true
		}
	}
	return false
}
