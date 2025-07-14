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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	NameDefaultInterface = "default"
)

type InterfaceSpec struct {
	Type          string `json:"type"`
	Name          string `json:"name"`
	InterfaceName string `json:"ifName"`
}

type InterfaceStatus struct {
	Type       string             `json:"type"`
	Name       string             `json:"name"`
	IfName     string             `json:"ifName"`
	Mac        string             `json:"mac"`
	IPAddress  string             `json:"ipAddress,omitempty"`
	Conditions []metav1.Condition `json:"conditions"`
}

type InterfaceSpecList []InterfaceSpec

func CreateNetworkSpec(vmSpec virtv2.VirtualMachineSpec) InterfaceSpecList {
	var networksSpec InterfaceSpecList
	for id, network := range vmSpec.Networks {
		if network.Type == virtv2.NetworksTypeMain {
			continue
		}

		networksSpec = append(networksSpec, InterfaceSpec{
			Type:          network.Type,
			Name:          network.Name,
			InterfaceName: generateInterfaceName(id, network.Type),
		})
	}

	return networksSpec
}

func (c InterfaceSpecList) ToString() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func generateInterfaceName(id int, networkType string) string {
	name := ""

	hash := md5.Sum([]byte(fmt.Sprintf("%d", id)))
	hashHex := hex.EncodeToString(hash[:])

	switch networkType {
	case virtv2.NetworksTypeNetwork:
		name = fmt.Sprintf("veth_n%s", hashHex[:8])
	case virtv2.NetworksTypeClusterNetwork:
		name = fmt.Sprintf("veth_cn%s", hashHex[:8])
	}
	return name
}
