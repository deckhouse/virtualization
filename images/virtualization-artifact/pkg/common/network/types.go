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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const NameDefaultInterface = "default"

// IPAssignmentModeDHCP is the ipAssignmentMode value indicating that SDN should
// raise a DHCP server on the tap interface to deliver the allocated IP address
// into the guest OS.
const IPAssignmentModeDHCP = "DHCP"

// WillProvisionInterface reports whether the given additional network interface
// will be included in networks-spec (i.e. SDN will provision it and report
// status). This mirrors the skip logic in EnrichWithIPAM:
//   - network without a pool: provisioned as L2-only (unless ipAddressName is
//     set, which is a config error -> skip);
//   - network with a pool, static mode: provisioned only if the IPAddress exists;
//   - network with a pool, auto mode: provisioned only if the auto IPAddress
//     exists and is allocated.
//
// Used by NetworkInterfaceHandler to avoid waiting for SDN status on skipped
// interfaces (which would produce a misleading "waiting for SDN" message).
func WillProvisionInterface(ctx context.Context, c client.Client, namespace string, vm *v1alpha2.VirtualMachine, ns v1alpha2.NetworksSpec) (bool, error) {
	if ns.Type == v1alpha2.NetworksTypeMain {
		return false, nil
	}
	hasPool, err := HasIPAM(ctx, c, namespace, ns)
	if err != nil {
		return false, err
	}
	if !hasPool {
		// No pool: L2-only is provisioned unless ipAddressName is set (config error).
		return ns.IPAddressName == "", nil
	}
	// Pool exists.
	if ns.IPAddressName != "" {
		// Static: provisioned only if the IPAddress exists, matches the network,
		// is allocated, and not used by another VM.
		exists, err := SDNIPAddressExists(ctx, c, namespace, ns.IPAddressName, ns.Type, ns.Name)
		if err != nil || !exists {
			return false, err
		}
		status, err := GetSDNIPAddressStatus(ctx, c, namespace, ns.IPAddressName)
		if err != nil || status == nil || !status.Allocated {
			return false, err
		}
		conflictVM, err := IsIPAddressNameUsedByAnotherVM(ctx, c, vm, ns.IPAddressName, ns)
		if err != nil || conflictVM != "" {
			return false, err
		}
		return true, nil
	}
	// Auto: provisioned only if the IPAddress exists and is allocated.
	name, err := FindSDNIPAddress(ctx, c, vm, ns)
	if err != nil || name == "" {
		return false, err
	}
	status, err := GetSDNIPAddressStatus(ctx, c, namespace, name)
	if err != nil || status == nil {
		return false, err
	}
	return status.Allocated, nil
}

// network has a pool configured (spec.ipam.ipAddressPoolRef), and resolves
// ipAddressNames:
//   - auto-mode (no user ipAddressName): finds the SDN IPAddress created by the
//     IPAddressHandler (by VM label) and includes its name. If the IPAddress is
//     not yet allocated (Pending/NoFreeIPAddress) or not found, the interface is
//     skipped (not included in the result) so CNI does not fail.
//   - static-mode (user ipAddressName): validates the referenced IPAddress
//     exists. If not found, the interface is skipped.
//   - networks without a pool: kept as L2-only (no ipAssignmentMode).
//
// The Main network is never enriched and is always kept in the result.
//
// This must be applied everywhere the networks-spec annotation is built so that
// the annotation is consistent between the KVVM template (initial pod creation)
// and a direct pod patch (live reconfiguration).
func EnrichWithIPAM(ctx context.Context, c client.Client, namespace string, vm *v1alpha2.VirtualMachine, specs InterfaceSpecList) (InterfaceSpecList, error) {
	result := make(InterfaceSpecList, 0, len(specs))
	for _, spec := range specs {
		if spec.Type == v1alpha2.NetworksTypeMain {
			result = append(result, spec)
			continue
		}
		hasPool, err := HasIPAM(ctx, c, namespace, v1alpha2.NetworksSpec{
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
		if len(spec.IPAddressNames) > 0 {
			// Static mode: validate the user-provided IPAddress exists and is allocated.
			exists, err := SDNIPAddressExists(ctx, c, namespace, spec.IPAddressNames[0], spec.Type, spec.Name)
			if err != nil {
				return nil, fmt.Errorf("check static IPAddress %s: %w", spec.IPAddressNames[0], err)
			}
			if !exists {
				continue
			}
			status, err := GetSDNIPAddressStatus(ctx, c, namespace, spec.IPAddressNames[0])
			if err != nil {
				return nil, fmt.Errorf("get status for static IPAddress %s: %w", spec.IPAddressNames[0], err)
			}
			if status == nil || !status.Allocated {
				continue
			}
			conflictVM, err := IsIPAddressNameUsedByAnotherVM(ctx, c, vm, spec.IPAddressNames[0], v1alpha2.NetworksSpec{Type: spec.Type, Name: spec.Name})
			if err != nil {
				return nil, fmt.Errorf("check static IPAddress %s conflict: %w", spec.IPAddressNames[0], err)
			}
			if conflictVM != "" {
				continue
			}
		} else {
			// Auto mode: find the IPAddress created by IPAddressHandler.
			name, err := FindSDNIPAddress(ctx, c, vm, v1alpha2.NetworksSpec{Type: spec.Type, Name: spec.Name})
			if err != nil {
				return nil, fmt.Errorf("find auto IPAddress for %s: %w", spec.Name, err)
			}
			if name == "" {
				continue
			}
			status, err := GetSDNIPAddressStatus(ctx, c, namespace, name)
			if err != nil {
				return nil, fmt.Errorf("get status for IPAddress %s: %w", name, err)
			}
			if status == nil || !status.Allocated {
				continue
			}
			spec.IPAddressNames = []string{name}
		}
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

func HasMainNetworkSpec(networks []v1alpha2.NetworksSpec) bool {
	return GetMainNetworkSpec(networks) != nil
}

func GetMainNetworkSpec(networks []v1alpha2.NetworksSpec) *v1alpha2.NetworksSpec {
	for i := range networks {
		if networks[i].Type == v1alpha2.NetworksTypeMain {
			return &networks[i]
		}
	}
	return nil
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

func (s InterfaceSpecList) ToString() (string, error) {
	filtered := InterfaceSpecList{}
	for _, spec := range s {
		if spec.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		filtered = append(filtered, spec)
	}

	data, err := json.Marshal(filtered)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
