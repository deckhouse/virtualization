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

import "github.com/deckhouse/virtualization/api/core/v1alpha2"

type MacAddressPool struct {
	reservedByName map[string]string
	available      []string
}

func NewMacAddressPool(vm *v1alpha2.VirtualMachine, vmmacs []*v1alpha2.VirtualMachineMACAddress) *MacAddressPool {
	reservedByName := make(map[string]string)
	takenMacs := make(map[string]bool)

	for _, n := range vm.Status.Networks {
		if n.Type != v1alpha2.NetworksTypeMain && n.MAC != "" {
			reservedByName[n.Name] = n.MAC
			takenMacs[n.MAC] = true
		}
	}

	var available []string
	for _, v := range vmmacs {
		mac := v.Status.Address
		if mac != "" && !takenMacs[mac] {
			available = append(available, mac)
		}
	}

	return &MacAddressPool{
		reservedByName: reservedByName,
		available:      available,
	}
}

func (p *MacAddressPool) Assign(networkName string) string {
	if mac, exists := p.reservedByName[networkName]; exists {
		return mac
	}

	if len(p.available) > 0 {
		mac := p.available[0]
		p.available = p.available[1:]
		return mac
	}

	return ""
}
