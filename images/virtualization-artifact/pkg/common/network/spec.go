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
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// WithImplicitMain writes out the Main network of a virtual machine listing none. Callers
// that filter the list must default before filtering, or "nothing is Ready" turns back into
// "nothing was asked for" and the machine lands on the pod network.
func WithImplicitMain(networks []v1alpha2.NetworksSpec) []v1alpha2.NetworksSpec {
	if len(networks) > 0 {
		return networks
	}
	return []v1alpha2.NetworksSpec{{
		Type: v1alpha2.NetworksTypeMain,
		ID:   ptr.To(ReservedMainID),
	}}
}

// CreateNetworkSpec builds the interface list for the given networks of the virtual machine.
// An empty list yields no interface at all, never the Main one.
func CreateNetworkSpec(vm *v1alpha2.VirtualMachine, networks []v1alpha2.NetworksSpec, vmmacs []*v1alpha2.VirtualMachineMACAddress) InterfaceSpecList {
	macPool := NewMacAddressPool(vm, vmmacs)
	var specs InterfaceSpecList

	for _, net := range networks {
		if net.Type == v1alpha2.NetworksTypeMain {
			specs = append(specs, createMainInterfaceSpec(net))
			continue
		}

		mac := macPool.Assign(net.Name)
		if mac != "" {
			specs = append(specs, createAdditionalInterfaceSpec(net, mac))
		}
	}

	return specs
}

const deckhouseUID = 64535

func createMainInterfaceSpec(net v1alpha2.NetworksSpec) InterfaceSpec {
	return InterfaceSpec{
		ID:            ptr.Deref(net.ID, 0),
		Type:          net.Type,
		Name:          net.Name,
		InterfaceName: NameDefaultInterface,
		MAC:           "",
		UID:           deckhouseUID,
		GID:           deckhouseUID,
	}
}

func createAdditionalInterfaceSpec(net v1alpha2.NetworksSpec, mac string) InterfaceSpec {
	spec := InterfaceSpec{
		ID:            ptr.Deref(net.ID, 0),
		Type:          net.Type,
		Name:          net.Name,
		InterfaceName: generateInterfaceName(mac, net.Type),
		MAC:           mac,
		UID:           deckhouseUID,
		GID:           deckhouseUID,
	}
	if net.IPAddressName != "" {
		spec.IPAddressNames = []string{net.IPAddressName}
	}
	if net.Type == v1alpha2.NetworksTypeUnderlayNetwork {
		spec.BindingMode = BindingModeVFIOPCI
		spec.VLANID = net.VLANID
		spec.VFMAC = mac
	}
	return spec
}

func generateInterfaceName(macAddress, networkType string) string {
	name := ""

	hash := md5.Sum([]byte(macAddress))
	hashHex := hex.EncodeToString(hash[:])

	switch networkType {
	case v1alpha2.NetworksTypeNetwork:
		name = fmt.Sprintf("veth_n%s", hashHex[:8])
	case v1alpha2.NetworksTypeClusterNetwork:
		name = fmt.Sprintf("veth_cn%s", hashHex[:8])
	}
	return name
}
