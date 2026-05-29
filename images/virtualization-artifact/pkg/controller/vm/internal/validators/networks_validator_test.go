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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
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
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(1)}}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(2)}}, true, true},
		{[]v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(1)},
			{Type: v1alpha2.NetworksTypeNetwork, Name: "test1", ID: ptr.To(2)},
			{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "test2", ID: ptr.To(3)},
		}, true, true},
		{[]v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(1)},
			{Type: v1alpha2.NetworksTypeNetwork, Name: "test1", ID: ptr.To(1)},
			{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "test2", ID: ptr.To(2)},
		}, true, false},
		{[]v1alpha2.NetworksSpec{
			{Type: v1alpha2.NetworksTypeNetwork, Name: "a", ID: ptr.To(2)},
			{Type: v1alpha2.NetworksTypeNetwork, Name: "b", ID: ptr.To(2)},
		}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(16383)}}, true, true},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(0)}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(16384)}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(-1)}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(2)}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(16383)}}, true, false},
		{[]v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(16384)}}, true, false},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("CreateTestCase%d", i), func(t *testing.T) {
			vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: test.networks}}

			// Create feature gate with SDN
			featureGate, _, setFromMap, err := featuregates.New()
			if err != nil {
				t.Fatalf("featuregates.New: %v", err)
			}
			if test.sdnEnabled {
				if err := setFromMap(map[string]bool{string(featuregates.SDN): true}); err != nil {
					t.Fatalf("setFromMap: %v", err)
				}
			}
			networkValidator := NewNetworksValidator(nil, featureGate)

			_, err = networkValidator.ValidateCreate(t.Context(), vm)
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
		phase           v1alpha2.MachinePhase
	}{
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(1)}},
			newNetworksSpec: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(2)}},
			sdnEnabled:      true,
			valid:           false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(1)}},
			newNetworksSpec: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(2)}},
			sdnEnabled:      true,
			valid:           true,
			phase:           v1alpha2.MachineStopped,
		},
		// nil → value is always allowed
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test"}},
			newNetworksSpec: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(1)}},
			sdnEnabled:      true,
			valid:           true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test"}},
			newNetworksSpec: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(1)}},
			sdnEnabled:      true,
			valid:           true,
			phase:           v1alpha2.MachineStopped,
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
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(1)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(2)},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(1)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(2)},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cluster", ID: ptr.To(5)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cluster", ID: ptr.To(10)},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(1)},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(2)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(1)},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(2)},
			},
			sdnEnabled: true,
			valid:      true,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(0)},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test1", ID: ptr.To(1)},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test2", ID: ptr.To(2)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(0)},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test1", ID: ptr.To(1)},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test2", ID: ptr.To(3)},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(0)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(0)},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "new", ID: ptr.To(5)},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(0)},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(1)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: ptr.To(0)},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(0)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(1)},
			},
			sdnEnabled: true,
			valid:      false,
		},
		{
			oldNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(1)},
			},
			newNetworksSpec: []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "test", ID: ptr.To(0)},
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
				Status: v1alpha2.VirtualMachineStatus{
					Phase: test.phase,
				},
			}

			// Create feature gate with SDN
			featureGate, _, setFromMap, err := featuregates.New()
			if err != nil {
				t.Fatalf("featuregates.New: %v", err)
			}
			if test.sdnEnabled {
				if err := setFromMap(map[string]bool{
					string(featuregates.SDN): true,
				}); err != nil {
					t.Fatalf("setFromMap: %v", err)
				}
			}
			networkValidator := NewNetworksValidator(nil, featureGate)
			_, err = networkValidator.ValidateUpdate(t.Context(), oldVM, newVM)

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

func newUnstructured(gvk schema.GroupVersionKind, name, namespace string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)
	if namespace != "" {
		u.SetNamespace(namespace)
	}
	return u
}

func TestNetworksValidatesExistence(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(commonnetwork.ClusterNetworkGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(commonnetwork.NetworkGVK, &unstructured.Unstructured{})

	existingCN := newUnstructured(commonnetwork.ClusterNetworkGVK, "exists-cn", "")
	existingNet := newUnstructured(commonnetwork.NetworkGVK, "exists-net", "default")

	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingCN, existingNet).
		Build()

	featureGate, _, setFromMap, err := featuregates.New()
	if err != nil {
		t.Fatalf("featuregates.New: %v", err)
	}
	if err := setFromMap(map[string]bool{string(featuregates.SDN): true}); err != nil {
		t.Fatalf("setFromMap: %v", err)
	}
	v := NewNetworksValidator(cli, featureGate)

	t.Run("create: missing networks are allowed (no existence check)", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{}
		vm.Namespace = "default"
		vm.Spec.Networks = []v1alpha2.NetworksSpec{mainNetwork, {Type: v1alpha2.NetworksTypeClusterNetwork, Name: "ghost"}}
		if _, err := v.ValidateCreate(t.Context(), vm); err != nil {
			t.Fatalf("ValidateCreate must not check network existence; got: %v", err)
		}
	})

	t.Run("update: adding existing ClusterNetwork passes", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{}
		oldVM.Namespace = "default"
		oldVM.Spec.Networks = []v1alpha2.NetworksSpec{mainNetwork}
		newVM := oldVM.DeepCopy()
		newVM.Spec.Networks = append(newVM.Spec.Networks, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "exists-cn"})
		if _, err := v.ValidateUpdate(t.Context(), oldVM, newVM); err != nil {
			t.Fatalf("expected no error; got: %v", err)
		}
	})

	t.Run("update: adding existing Network passes", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{}
		oldVM.Namespace = "default"
		oldVM.Spec.Networks = []v1alpha2.NetworksSpec{mainNetwork}
		newVM := oldVM.DeepCopy()
		newVM.Spec.Networks = append(newVM.Spec.Networks, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "exists-net"})
		if _, err := v.ValidateUpdate(t.Context(), oldVM, newVM); err != nil {
			t.Fatalf("expected no error; got: %v", err)
		}
	})

	t.Run("update: existing networks are not re-checked", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{}
		oldVM.Namespace = "default"
		oldVM.Spec.Networks = []v1alpha2.NetworksSpec{mainNetwork, {Type: v1alpha2.NetworksTypeClusterNetwork, Name: "ghost"}}
		newVM := oldVM.DeepCopy()
		// Add another non-Main network; existing "ghost" should not be re-checked.
		newVM.Spec.Networks = append(newVM.Spec.Networks, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "exists-cn"})
		if _, err := v.ValidateUpdate(t.Context(), oldVM, newVM); err != nil {
			t.Fatalf("expected no error for adding an existing network when prior spec had a missing one; got: %v", err)
		}
	})

	t.Run("update: adding a missing network is rejected", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{}
		oldVM.Namespace = "default"
		oldVM.Spec.Networks = []v1alpha2.NetworksSpec{mainNetwork, {Type: v1alpha2.NetworksTypeClusterNetwork, Name: "exists-cn"}}
		newVM := oldVM.DeepCopy()
		newVM.Spec.Networks = append(newVM.Spec.Networks, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "ghost"})
		if _, err := v.ValidateUpdate(t.Context(), oldVM, newVM); err == nil {
			t.Fatalf("expected error when adding a missing network")
		}
	})
}
