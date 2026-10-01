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
	"slices"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type MacAddressPool struct {
	macByNetwork map[string]string
}

// NewMacAddressPool binds a MAC address to every additional network of the spec, so that the
// KVVM template, the pod annotation and the status agree whichever networks they render.
func NewMacAddressPool(vm *v1alpha2.VirtualMachine, vmmacs []*v1alpha2.VirtualMachineMACAddress, kvvmExists bool) *MacAddressPool {
	macByNetwork := make(map[string]string)
	taken := make(map[string]bool)
	bind := func(networkType, networkName, mac string) {
		key := poolKey(networkType, networkName)
		if _, ok := macByNetwork[key]; ok || mac == "" || taken[mac] {
			return
		}
		macByNetwork[key] = mac
		taken[mac] = true
	}

	addressByVMMACName := make(map[string]string, len(vmmacs))
	for _, v := range vmmacs {
		if v.Status.Address != "" {
			addressByVMMACName[v.Name] = v.Status.Address
		}
	}
	bindStatus := func() {
		for _, n := range vm.Status.Networks {
			if n.Type != v1alpha2.NetworksTypeMain {
				bind(n.Type, n.Name, n.MAC)
			}
		}
	}
	bindPins := func() {
		for _, n := range vm.Spec.Networks {
			if n.Type != v1alpha2.NetworksTypeMain && n.VirtualMachineMACAddressName != "" {
				bind(n.Type, n.Name, addressByVMMACName[n.VirtualMachineMACAddressName])
			}
		}
	}
	// A running machine keeps the addresses of its status. Without a KVVM the pins win: a restore
	// deletes the KVVM and pins the captured addresses while the status still holds the old ones.
	if kvvmExists {
		bindStatus()
		bindPins()
	} else {
		bindPins()
		bindStatus()
	}

	var free []string
	for _, v := range vmmacs {
		if mac := v.Status.Address; mac != "" && !taken[mac] {
			free = append(free, mac)
		}
	}
	// The listing comes in no particular order, and each caller builds its own pool.
	slices.Sort(free)
	for _, n := range vm.Spec.Networks {
		if n.Type != v1alpha2.NetworksTypeMain && len(free) > 0 {
			key := poolKey(n.Type, n.Name)
			if _, ok := macByNetwork[key]; !ok {
				macByNetwork[key] = free[0]
				free = free[1:]
			}
		}
	}

	return &MacAddressPool{macByNetwork: macByNetwork}
}

func (p *MacAddressPool) MACFor(networkType, networkName string) string {
	return p.macByNetwork[poolKey(networkType, networkName)]
}

func poolKey(networkType, networkName string) string {
	return networkType + "/" + networkName
}
