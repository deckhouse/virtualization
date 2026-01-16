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

func CreateNetworkSpec(vm *v1alpha2.VirtualMachine, vmmacs []*v1alpha2.VirtualMachineMACAddress) InterfaceSpecList {
	var (
		all     []string
		status  []struct{ Name, MAC string }
		taken   = make(map[string]bool)
		free    []string
		res     InterfaceSpecList
		freeIdx int
	)

	for _, v := range vmmacs {
		if mac := v.Status.Address; mac != "" {
			all = append(all, mac)
		}
	}
	for _, n := range vm.Status.Networks {
		if n.Type == v1alpha2.NetworksTypeMain {
			continue
		}
		status = append(status, struct{ Name, MAC string }{n.Name, n.MAC})
		taken[n.MAC] = true
	}
	for _, mac := range all {
		if !taken[mac] {
			free = append(free, mac)
		}
	}
	for _, n := range vm.Spec.Networks {
		if n.Type == v1alpha2.NetworksTypeMain {
			res = append(res, InterfaceSpec{
				Type:          n.Type,
				Name:          n.Name,
				InterfaceName: NameDefaultInterface,
				MAC:           "",
			})
			continue
		}
		var mac string
		for i, s := range status {
			if s.Name == n.Name {
				mac = s.MAC
				status = append(status[:i], status[i+1:]...)
				break
			}
		}
		if mac == "" && freeIdx < len(free) {
			mac = free[freeIdx]
			freeIdx++
		}
		if mac != "" {
			res = append(res, InterfaceSpec{
				Type:          n.Type,
				Name:          n.Name,
				InterfaceName: generateInterfaceName(mac, n.Type),
				MAC:           mac,
			})
		}
	}
	return res
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
