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
	"encoding/json"

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
}

type InterfaceStatus struct {
	Type       string             `json:"type"`
	Name       string             `json:"name"`
	IfName     string             `json:"ifName"`
	Mac        string             `json:"mac"`
	Conditions []metav1.Condition `json:"conditions"`
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
