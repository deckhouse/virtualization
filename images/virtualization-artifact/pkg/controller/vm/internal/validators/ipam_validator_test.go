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

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
