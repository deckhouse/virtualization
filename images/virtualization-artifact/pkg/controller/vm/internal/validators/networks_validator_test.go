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

package validators

import (
	"fmt"
	"testing"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestNetworksValidate(t *testing.T) {
	tests := []struct {
		networks   []v1alpha2.NetworksSpec
		sdnEnabled bool
		valid      bool
	}{
		{[]v1alpha2.NetworksSpec{}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}, {Type: v1alpha2.NetworksTypeMain}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, Name: "main"}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test"}, {Type: v1alpha2.NetworksTypeMain}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}, {Type: v1alpha2.NetworksTypeNetwork, Name: "test"}}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}, {Type: v1alpha2.NetworksTypeNetwork}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}, false, false},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("TestCase%d", i), func(t *testing.T) {
			vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: test.networks}}

			// Create feature gate with SDN
			featureGate, _, setFromMap, _ := featuregates.New()
			if test.sdnEnabled {
				_ = setFromMap(map[string]bool{string(featuregates.SDN): true})
			}
			networkValidator := NewNetworksValidator(featureGate)

			_, err := networkValidator.Validate(vm)
			if test.valid && err != nil {
				t.Errorf("For spec %s expected valid, but validation failed", test.networks)
			}

			if !test.valid && err == nil {
				t.Errorf("For spec %s expected not valid, but validation succeeded", test.networks)
			}
		})
	}
}
