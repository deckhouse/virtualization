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

package validators

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestIPAMValidatorValidateCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}

	newValidator := func(virtualMachineCIDRs []string) *IPAMValidator {
		return NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).Build(), virtualMachineCIDRs)
	}

	vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{VirtualMachineIPAddress: "vmip", Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}}}
	if _, err := newValidator(nil).ValidateCreate(t.Context(), vm); err == nil {
		t.Fatalf("expected error without CIDRs")
	}

	vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test"}}
	if _, err := newValidator([]string{"10.0.0.0/24"}).ValidateCreate(t.Context(), vm); err == nil {
		t.Fatalf("expected error without Main network")
	}

	vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}
	if _, err := newValidator([]string{"10.0.0.0/24"}).ValidateCreate(t.Context(), vm); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestIPAMValidatorValidateUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}

	validator := NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).Build(), nil)

	oldVM := &v1alpha2.VirtualMachine{}
	newVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{VirtualMachineIPAddress: "vmip", Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}}}
	if _, err := validator.ValidateUpdate(t.Context(), oldVM, newVM); err == nil {
		t.Fatalf("expected error without CIDRs")
	}
}

// Regression: a VM that already has spec.virtualMachineIPAddressName set (e.g. from
// before virtualMachineCIDRs was removed from ModuleConfig) must still accept
// unrelated updates - metadata (e.g. finalizers) or status. Only an actual change to
// spec.virtualMachineIPAddressName or spec.networks should be gated by the CIDRs check.
func TestIPAMValidatorValidateUpdateAllowsUnrelatedChangesWithoutCIDRs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}

	validator := NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).Build(), nil)

	oldVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{VirtualMachineIPAddress: "vmip", Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}}}

	t.Run("unrelated metadata change is allowed on a VM already having a static IP without CIDRs", func(t *testing.T) {
		newVM := oldVM.DeepCopy()
		newVM.Finalizers = []string{"test-finalizer"}
		if _, err := validator.ValidateUpdate(t.Context(), oldVM, newVM); err != nil {
			t.Fatalf("expected success for an unrelated update on a grandfathered VM without CIDRs, got: %v", err)
		}
	})

	t.Run("changing spec.networks while keeping the same IP is still rejected without CIDRs", func(t *testing.T) {
		newVM := oldVM.DeepCopy()
		newVM.Spec.Networks = append(newVM.Spec.Networks, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "extra"})
		if _, err := validator.ValidateUpdate(t.Context(), oldVM, newVM); err == nil {
			t.Fatalf("expected error when spec.networks changes without CIDRs")
		}
	})
}

func TestIPAMValidatorMainIPAddressName(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}
	if err := virtv1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme virtv1: %v", err)
	}
	_ = sdnapi.AddToScheme(scheme)

	newIPAddress := func(name, networkKind, networkName string) *sdnapi.IPAddress {
		return &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
			Spec:       sdnapi.IPAddressSpec{NetworkRef: sdnapi.NetworkRef{Kind: networkKind, Name: networkName}},
		}
	}

	newMainNetwork := func() *unstructured.Unstructured {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(commonnetwork.ClusterNetworkGVK)
		u.SetName(commonnetwork.MainClusterNetworkName)
		_ = unstructured.SetNestedField(u.Object, "PrimaryCNI", "spec", "type")
		_ = unstructured.SetNestedField(u.Object, map[string]any{
			"ipAddressPoolRef": map[string]any{"kind": "ClusterIPAddressPool", "name": "d8-virtualization"},
		}, "spec", "ipam")
		return u
	}

	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "my-ip"}},
			},
		}
	}

	newValidator := func(objs ...client.Object) *IPAMValidator {
		return NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(), []string{"10.0.0.0/24"})
	}

	// Trigger inactive: the field would be silently ignored -> reject.
	if _, err := newValidator().ValidateCreate(t.Context(), newVM()); err == nil {
		t.Fatalf("expected error when the sdn module does not serve the Main network")
	}

	// An IPAddress of another network: the VM would wait in Pending forever.
	foreign := newIPAddress("my-ip", "Network", "other")
	if _, err := newValidator(newMainNetwork(), foreign).ValidateCreate(t.Context(), newVM()); err == nil {
		t.Fatalf("expected error when the referenced IPAddress belongs to another network")
	}

	// The very same name in the Main network: accepted.
	own := newIPAddress("my-ip", "ClusterNetwork", commonnetwork.MainClusterNetworkName)
	if _, err := newValidator(newMainNetwork(), own).ValidateCreate(t.Context(), newVM()); err != nil {
		t.Fatalf("expected success for an IPAddress of the Main network, got: %v", err)
	}

	// New VM (no internal VM yet) with the active trigger: allowed.
	if _, err := newValidator(newMainNetwork()).ValidateCreate(t.Context(), newVM()); err != nil {
		t.Fatalf("expected success for a new VM, got: %v", err)
	}

	// The same field added to a machine that already runs: allowed too.
	kvvm := &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"}}
	oldVM := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"}}
	if _, err := newValidator(newMainNetwork(), kvvm).ValidateUpdate(t.Context(), oldVM, newVM()); err != nil {
		t.Fatalf("expected success for a running VM, got: %v", err)
	}

	// The pool is gone: a machine keeps its named IPAddress through other network changes...
	withExtraNetwork := newVM()
	withExtraNetwork.Spec.Networks = append(withExtraNetwork.Spec.Networks, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, Name: "extra"})
	if _, err := newValidator().ValidateUpdate(t.Context(), newVM(), withExtraNetwork); err != nil {
		t.Fatalf("expected success for a change of other networks without the pool, got: %v", err)
	}

	// ...but cannot name another one.
	renamed := newVM()
	renamed.Spec.Networks[0].IPAddressName = "other-ip"
	if _, err := newValidator().ValidateUpdate(t.Context(), newVM(), renamed); err == nil {
		t.Fatalf("expected error for a new ipAddressName of the Main network without the pool")
	}
}

// The deprecation warning points to spec.networks[Main].ipAddressName, so it is
// only useful where the sdn module can serve that address.
func TestIPAMValidatorDeprecationWarningFollowsTheTrigger(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}
	_ = sdnapi.AddToScheme(scheme)

	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"},
		Spec: v1alpha2.VirtualMachineSpec{
			VirtualMachineIPAddress: "vmip",
			Networks:                []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}},
		},
	}

	silent := NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).Build(), []string{"10.0.0.0/24"})
	warnings, err := silent.ValidateCreate(t.Context(), vm)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warning without the sdn module, got %v", warnings)
	}

	mainNetwork := &unstructured.Unstructured{}
	mainNetwork.SetGroupVersionKind(commonnetwork.ClusterNetworkGVK)
	mainNetwork.SetName(commonnetwork.MainClusterNetworkName)
	_ = unstructured.SetNestedField(mainNetwork.Object, "PrimaryCNI", "spec", "type")
	_ = unstructured.SetNestedField(mainNetwork.Object, map[string]any{
		"ipAddressPoolRef": map[string]any{"kind": "ClusterIPAddressPool", "name": "d8-virtualization"},
	}, "spec", "ipam")

	loud := NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(mainNetwork).Build(), []string{"10.0.0.0/24"})
	warnings, err = loud.ValidateCreate(t.Context(), vm)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected the deprecation warning, got %v", warnings)
	}
}

// A vmip named after the machine used to demand a reference to it. A machine the sdn
// module serves gets an IPAddress and never binds that vmip.
func TestIPAMValidatorVMIPNamedAfterTheMachine(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}
	_ = sdnapi.AddToScheme(scheme)

	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"}}
	vmip := &v1alpha2.VirtualMachineIPAddress{ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "ns"}}

	legacy := NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmip).Build(), []string{"10.0.0.0/24"})
	if _, err := legacy.ValidateCreate(t.Context(), vm); err == nil {
		t.Fatalf("expected error: the vmip named after the machine must be referenced")
	}

	mainNetwork := &unstructured.Unstructured{}
	mainNetwork.SetGroupVersionKind(commonnetwork.ClusterNetworkGVK)
	mainNetwork.SetName(commonnetwork.MainClusterNetworkName)
	_ = unstructured.SetNestedField(mainNetwork.Object, "PrimaryCNI", "spec", "type")
	_ = unstructured.SetNestedField(mainNetwork.Object, map[string]any{
		"ipAddressPoolRef": map[string]any{"kind": "ClusterIPAddressPool", "name": "d8-virtualization"},
	}, "spec", "ipam")

	sdn := NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).WithObjects(vmip, mainNetwork).Build(), []string{"10.0.0.0/24"})
	if _, err := sdn.ValidateCreate(t.Context(), vm); err != nil {
		t.Fatalf("expected success where the sdn module serves the Main network, got: %v", err)
	}
}
