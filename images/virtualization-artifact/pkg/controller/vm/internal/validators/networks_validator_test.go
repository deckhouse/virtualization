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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestNetworksValidate(t *testing.T) {
	tests := []struct {
		networks []virtv2.NetworksSpec
		valid    bool
	}{
		{[]virtv2.NetworksSpec{}, true},
		{[]virtv2.NetworksSpec{{Type: virtv2.NetworksTypeMain}}, true},
		{[]virtv2.NetworksSpec{{Type: virtv2.NetworksTypeMain}, {Type: virtv2.NetworksTypeMain}}, false},
		{[]virtv2.NetworksSpec{{Type: virtv2.NetworksTypeMain, Name: "main"}}, false},
		{[]virtv2.NetworksSpec{{Type: virtv2.NetworksTypeNetwork, Name: "test"}, {Type: virtv2.NetworksTypeMain}}, false},
		{[]virtv2.NetworksSpec{{Type: virtv2.NetworksTypeMain}, {Type: virtv2.NetworksTypeNetwork, Name: "test"}}, true},
		{[]virtv2.NetworksSpec{{Type: virtv2.NetworksTypeMain}, {Type: virtv2.NetworksTypeNetwork}}, false},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("TestCase%d", i), func(t *testing.T) {
			vm := &virtv2.VirtualMachine{Spec: virtv2.VirtualMachineSpec{Networks: test.networks}}
			networkValidator := NewNetworksValidator()

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
