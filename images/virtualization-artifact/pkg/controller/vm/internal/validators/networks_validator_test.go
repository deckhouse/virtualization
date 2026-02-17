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

var (
	mainNetwork        = v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeMain}
	networkTest        = v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "test"}
	clusterNetworkTest = v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "test"}
)

func TestNetworksValidateCreate(t *testing.T) {
	tests := []struct {
		networks   []v1alpha2.NetworksSpec
		sdnEnabled bool
		valid      bool
	}{
		{[]v1alpha2.NetworksSpec{}, true, true},
		{[]v1alpha2.NetworksSpec{mainNetwork}, true, true},
		{[]v1alpha2.NetworksSpec{mainNetwork, mainNetwork}, true, false},
		{[]v1alpha2.NetworksSpec{networkTest, mainNetwork, mainNetwork}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, Name: "main"}}, true, false},
		{[]v1alpha2.NetworksSpec{networkTest}, true, true},
		{[]v1alpha2.NetworksSpec{networkTest, clusterNetworkTest}, true, true},
		{[]v1alpha2.NetworksSpec{mainNetwork, networkTest}, true, true},
		{[]v1alpha2.NetworksSpec{mainNetwork, networkTest, networkTest}, true, false},
		{[]v1alpha2.NetworksSpec{mainNetwork, {Type: v1alpha2.NetworksTypeNetwork}}, true, false},
		{[]v1alpha2.NetworksSpec{mainNetwork}, false, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: 1}}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 2}}, true, true},
		{[]v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeMain, ID: 1},
			{Type: v1alpha2.NetworksTypeNetwork, Name: "test1", ID: 1},
			{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "test2", ID: 2},
		}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 16383}}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 0}}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 16384}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: -1}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: 16383}}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: 16384}}, true, false},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("CreateTestCase%d", i), func(t *testing.T) {
			vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: test.networks}}

			// Create feature gate with SDN
			featureGate, _, setFromMap, _ := featuregates.New()
			if test.sdnEnabled {
				_ = setFromMap(map[string]bool{string(featuregates.SDN): true})
			}
			networkValidator := NewNetworksValidator(featureGate)

			_, err := networkValidator.ValidateCreate(t.Context(), vm)
			if test.valid && err != nil {
				t.Errorf("Validation failed for spec %v: expected valid, but got an error: %v", test.networks, err)
			}
			if !test.valid && err == nil {
				t.Errorf("Validation succeeded for spec %v: expected error, but got none", test.networks)
			}
		})
	}
}

func TestNetworksValidateUpdate(t *testing.T) {
	tests := []struct {
		oldNetworksSpec []v1alpha2.NetworksSpec
		newNetworksSpec []v1alpha2.NetworksSpec
		sdnEnabled      bool
		valid           bool
	}{
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{},
			newNetworksSpec: []v1alpha2.NetworksSpec{},
			sdnEnabled:      true,
			valid:           true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{},
			newNetworksSpec: []v1alpha2.NetworksSpec{mainNetwork},
			sdnEnabled:      true,
			valid:           true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				mainNetwork,
				networkTest,
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				mainNetwork,
				networkTest,
				networkTest,
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				mainNetwork,
				networkTest,
				networkTest,
				networkTest,
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				mainNetwork,
				networkTest,
				networkTest,
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				mainNetwork,
				networkTest,
				networkTest,
				networkTest,
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				mainNetwork,
				networkTest,
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				mainNetwork,
				networkTest,
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				networkTest,
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				networkTest,
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				mainNetwork,
				networkTest,
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 1},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 2},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 1},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 2},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cluster", ID: 5},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cluster", ID: 10},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 1},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 2},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 1},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 2},
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 0},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test1", ID: 1},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test2", ID: 2},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 0},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test1", ID: 1},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test2", ID: 3},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 0},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 0},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "new", ID: 5},
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 0},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 1},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: 0},
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 0},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 1},
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 1},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: 0},
			},
			sdnEnabled: true,
			valid:      false,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("UpdateTestCase%d", i), func(t *testing.T) {
			oldVM := &v1alpha2.VirtualMachine{
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: test.oldNetworksSpec,
				},
			}
			newVM := &v1alpha2.VirtualMachine{
				Spec: v1alpha2.VirtualMachineSpec{
					Networks: test.newNetworksSpec,
				},
			}

			// Create feature gate with SDN
			featureGate, _, setFromMap, _ := featuregates.New()
			if test.sdnEnabled {
				_ = setFromMap(map[string]bool{
					string(featuregates.SDN): true,
				})
			}
			networkValidator := NewNetworksValidator(featureGate)
			_, err := networkValidator.ValidateUpdate(t.Context(), oldVM, newVM)

			if test.valid && err != nil {
				t.Errorf(
					"Validation failed for old spec %v and new spec %v: expected valid, but got an error: %v",
					test.oldNetworksSpec, test.newNetworksSpec, err,
				)
			}
			if !test.valid && err == nil {
				t.Errorf(
					"Validation succeeded for old spec %v and new spec %v: expected error, but got none",
					test.oldNetworksSpec, test.newNetworksSpec,
				)
			}
		})
	}
}
