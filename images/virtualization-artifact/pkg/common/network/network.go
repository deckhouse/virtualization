/*
Copyright 2025 Flant JSC

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
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const NameDefaultInterface = "default"

func HasMainNetworkStatus(networks []v1alpha2.NetworksStatus) bool {
	for _, network := range networks {
		if network.Type == v1alpha2.NetworksTypeMain {
			return true
		}
	}

	return false
}

func HasMainNetworkSpec(networks []v1alpha2.NetworksSpec) bool {
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
}

type InterfaceStatus struct {
	Type       string             `json:"type"`
	Name       string             `json:"name"`
	IfName     string             `json:"ifName"`
	Mac        string             `json:"mac"`
	Conditions []metav1.Condition `json:"conditions"`
}

type InterfaceSpecList []InterfaceSpec

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

func CreateNetworkSpec(vm *v1alpha2.VirtualMachine, vmmacs []*v1alpha2.VirtualMachineMACAddress) InterfaceSpecList {
	macPool := NewMacAddressPool(vm, vmmacs)
	var specs InterfaceSpecList

	for _, net := range vm.Spec.Networks {
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

func createMainInterfaceSpec(net v1alpha2.NetworksSpec) InterfaceSpec {
	return InterfaceSpec{
		ID:            net.ID,
		Type:          net.Type,
		Name:          net.Name,
		InterfaceName: NameDefaultInterface,
		MAC:           "",
	}
}

func createAdditionalInterfaceSpec(net v1alpha2.NetworksSpec, mac string) InterfaceSpec {
	return InterfaceSpec{
		ID:            net.ID,
		Type:          net.Type,
		Name:          net.Name,
		InterfaceName: generateInterfaceName(mac, net.Type),
		MAC:           mac,
	}
}

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

const (
	ReservedMainID = 1
	StartGenericID = 2
)

type InterfaceIDAllocator struct {
	used   map[int]bool
	cursor int
}

func NewInterfaceIDAllocator() *InterfaceIDAllocator {
	return &InterfaceIDAllocator{
		used:   make(map[int]bool),
		cursor: StartGenericID,
	}
}

func (a *InterfaceIDAllocator) Reserve(id int) {
	if id > 0 {
		a.used[id] = true
	}
}

func (a *InterfaceIDAllocator) NextAvailable() int {
	for {
		if a.cursor == ReservedMainID {
			a.cursor++
			continue
		}

		if !a.used[a.cursor] {
			id := a.cursor
			a.used[id] = true
			a.cursor++
			return id
		}
		a.cursor++
	}
}
