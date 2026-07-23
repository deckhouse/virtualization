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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

type networkValidatorOpts struct {
	virtualMachineCIDRs []string
	objects             []client.Object
	sdnEnabled          bool
}

func newNetworksValidator(t *testing.T, opts networkValidatorOpts) *NetworksValidator {
	t.Helper()
	scheme := runtime.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(opts.objects) > 0 {
		builder = builder.WithObjects(opts.objects...)
	}

	featureGate, _, setFromMap, err := featuregates.New()
	if err != nil {
		t.Fatalf("featuregates.New: %v", err)
	}
	if opts.sdnEnabled {
		if err := setFromMap(map[string]bool{string(featuregates.SDN): true}); err != nil {
			t.Fatalf("setFromMap: %v", err)
		}
	}

	return NewNetworksValidator(builder.Build(), featureGate, opts.virtualMachineCIDRs)
}

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

			networkValidator := newNetworksValidator(
				t,
				networkValidatorOpts{
					virtualMachineCIDRs: []string{"10.0.0.0/24"},
					sdnEnabled:          test.sdnEnabled,
				},
			)

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
			valid:      false,
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

			networkValidator := newNetworksValidator(
				t,
				networkValidatorOpts{
					virtualMachineCIDRs: []string{"10.0.0.0/24"},
					sdnEnabled:          test.sdnEnabled,
				},
			)
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

func newUnstructured(gvk schema.GroupVersionKind, name, namespace string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetName(name)
	if namespace != "" {
		u.SetNamespace(namespace)
	}
	return u
}

// Regression: the isChanged short-circuit in ValidateUpdate relies on
// equality.Semantic.DeepEqual treating a reordered spec.networks as changed (slice
// comparisons are position-sensitive), so a reorder must still run full validation -
// including the "Main must be first" rule - rather than being waved through as a
// no-op. validateNetworkIDsUnchanged, by contrast, is keyed by network identifier
// (type/name) on purpose and does not care about position at all.
func TestNetworksValidateUpdateReordering(t *testing.T) {
	networkValidator := newNetworksValidator(t, networkValidatorOpts{
		virtualMachineCIDRs: []string{"10.0.0.0/24"},
		sdnEnabled:          true,
	})

	t.Run("moving Main out of the first position is rejected even though the set of networks is unchanged", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{mainNetwork, networkTest}}}
		newVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{networkTest, mainNetwork}}}
		if _, err := networkValidator.ValidateUpdate(t.Context(), oldVM, newVM); err == nil {
			t.Fatalf("expected error: reordering must not bypass the Main-must-be-first validation")
		}
	})

	t.Run("reordering two non-Main networks without changing IDs is allowed", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{mainNetwork, networkTest, clusterNetworkTest}}}
		newVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{mainNetwork, clusterNetworkTest, networkTest}}}
		if _, err := networkValidator.ValidateUpdate(t.Context(), oldVM, newVM); err != nil {
			t.Fatalf("expected reordering non-Main networks to be allowed, got: %v", err)
		}
	})
}

func TestNetworksValidatorMainRequiresCIDRs(t *testing.T) {
	newValidatorWithDCVROnly := newNetworksValidator(
		t,
		networkValidatorOpts{
			sdnEnabled: true,
		},
	)

	t.Run("create: explicit Main without CIDRs is rejected", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{mainNetwork}}}
		if _, err := newValidatorWithDCVROnly.ValidateCreate(t.Context(), vm); err == nil {
			t.Fatalf("expected error for explicit Main without CIDRs")
		}
	})

	t.Run("create: implicit Main without CIDRs is rejected", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{}
		if _, err := newValidatorWithDCVROnly.ValidateCreate(t.Context(), vm); err == nil {
			t.Fatalf("expected error for empty spec.networks without CIDRs")
		}
	})

	t.Run("update: newly setting explicit Main without CIDRs is rejected", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{networkTest}}}
		newVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{mainNetwork}}}
		if _, err := newValidatorWithDCVROnly.ValidateUpdate(t.Context(), oldVM, newVM); err == nil {
			t.Fatalf("expected update error for newly set explicit Main without CIDRs")
		}
	})

	t.Run("update: clearing spec.networks to implicit Main without CIDRs is rejected", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{networkTest}}}
		newVM := &v1alpha2.VirtualMachine{}
		if _, err := newValidatorWithDCVROnly.ValidateUpdate(t.Context(), oldVM, newVM); err == nil {
			t.Fatalf("expected update error for clearing spec.networks to implicit Main without CIDRs")
		}
	})

	// Regression: a VM created back when CIDRs were configured (or before the rule
	// existed) can end up with an explicit or implicit Main network that would now
	// be rejected on create. Once such a VM exists, unrelated updates - metadata
	// (e.g. finalizers) or status - must still go through; only actual spec.networks
	// changes are gated by the CIDRs check.
	t.Run("update: unrelated changes are allowed on a VM already having explicit Main without CIDRs", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{mainNetwork}}}
		newVM := oldVM.DeepCopy()
		newVM.Finalizers = []string{"test-finalizer"}
		if _, err := newValidatorWithDCVROnly.ValidateUpdate(t.Context(), oldVM, newVM); err != nil {
			t.Fatalf("expected success for an unrelated update on a grandfathered VM without CIDRs, got: %v", err)
		}
	})

	t.Run("update: unrelated changes are allowed on a VM already having implicit Main without CIDRs", func(t *testing.T) {
		oldVM := &v1alpha2.VirtualMachine{}
		newVM := oldVM.DeepCopy()
		newVM.Finalizers = []string{"test-finalizer"}
		if _, err := newValidatorWithDCVROnly.ValidateUpdate(t.Context(), oldVM, newVM); err != nil {
			t.Fatalf("expected success for an unrelated update on a grandfathered VM with implicit Main and no CIDRs, got: %v", err)
		}
	})

	networkValidator := newNetworksValidator(
		t,
		networkValidatorOpts{
			virtualMachineCIDRs: []string{"10.0.0.0/24"},
			sdnEnabled:          true,
		},
	)

	t.Run("create: explicit Main with CIDRs is allowed", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{Networks: []v1alpha2.NetworksSpec{mainNetwork}}}
		if _, err := networkValidator.ValidateCreate(t.Context(), vm); err != nil {
			t.Fatalf("expected success with CIDRs, got: %v", err)
		}
	})

	t.Run("create: empty spec.networks with CIDRs is allowed", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{}
		if _, err := networkValidator.ValidateCreate(t.Context(), vm); err != nil {
			t.Fatalf("expected success with CIDRs for empty spec.networks, got: %v", err)
		}
	})
}

func TestNetworksValidatesExistence(t *testing.T) {
	existingCN := newUnstructured(commonnetwork.ClusterNetworkGVK, "exists-cn", "")
	existingNet := newUnstructured(commonnetwork.NetworkGVK, "exists-net", "default")

	v := newNetworksValidator(
		t,
		networkValidatorOpts{
			virtualMachineCIDRs: []string{"10.0.0.0/24"},
			objects:             []client.Object{existingCN, existingNet},
			sdnEnabled:          true,
		},
	)

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
