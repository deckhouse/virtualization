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
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const coreFractionClassName = "class"

func newAutoscalerFeatureGate(t *testing.T, autoscaler, inPlaceResize bool) featuregate.FeatureGate {
	t.Helper()

	gate, setFromMap, err := featuregates.NewUnlocked()
	if err != nil {
		t.Fatalf("failed to create feature gate: %v", err)
	}

	err = setFromMap(map[string]bool{
		string(featuregates.VerticalVirtualMachineAutoscaler):     autoscaler,
		string(featuregates.HotplugCPUAndMemoryWithInPlaceResize): inPlaceResize,
	})
	if err != nil {
		t.Fatalf("failed to set feature gates: %v", err)
	}

	return gate
}

func vmWithCoreFraction(coreFraction string) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		Spec: v1alpha2.VirtualMachineSpec{
			VirtualMachineClassName: coreFractionClassName,
			CPU:                     v1alpha2.CPUSpec{Cores: 2, CoreFraction: coreFraction},
		},
	}
}

// coreFractionClient serves a VirtualMachineClass whose sizing policy for 1..8 cores
// allows the given core fractions. Without fractions the policy constrains nothing.
func coreFractionClient(t *testing.T, fractions ...v1alpha2.CoreFractionValue) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}

	class := &v1alpha2.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{Name: coreFractionClassName},
		Spec: v1alpha2.VirtualMachineClassSpec{
			SizingPolicies: []v1alpha2.SizingPolicy{{
				Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 8},
				CoreFractions: fractions,
			}},
		},
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(class).Build()
}

func TestCoreFractionValidate(t *testing.T) {
	tests := []struct {
		name          string
		coreFraction  string
		autoscaler    bool
		inPlaceResize bool
		valid         bool
	}{
		{"auto allowed when both features enabled", v1alpha2.CoreFractionAuto, true, true, true},
		{"auto rejected when autoscaler disabled", v1alpha2.CoreFractionAuto, false, true, false},
		{"auto rejected when in-place resize disabled", v1alpha2.CoreFractionAuto, true, false, false},
		{"auto rejected when both disabled", v1alpha2.CoreFractionAuto, false, false, false},
		{"explicit fraction always allowed", "50%", false, false, true},
		{"empty fraction always allowed", "", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := newAutoscalerFeatureGate(t, tt.autoscaler, tt.inPlaceResize)
			v := NewCoreFractionValidator(coreFractionClient(t, 25, 50), gate)
			vm := vmWithCoreFraction(tt.coreFraction)

			if _, err := v.ValidateCreate(context.Background(), vm); (err == nil) != tt.valid {
				t.Errorf("ValidateCreate: got err %v, want valid=%v", err, tt.valid)
			}
			// An update whose old value is not "Auto" is treated like a create: the
			// transition into "Auto" is gated the same way.
			oldVM := vmWithCoreFraction("50%")
			if _, err := v.ValidateUpdate(context.Background(), oldVM, vm); (err == nil) != tt.valid {
				t.Errorf("ValidateUpdate: got err %v, want valid=%v", err, tt.valid)
			}
		})
	}
}

// TestCoreFractionSizingPolicy verifies that "Auto" is only accepted when the sizing
// policy of the VM's class leaves the autoscaler more than one core fraction to pick
// from. 100% never counts: it is not used automatically.
func TestCoreFractionSizingPolicy(t *testing.T) {
	tests := []struct {
		name      string
		fractions []v1alpha2.CoreFractionValue
		valid     bool
	}{
		{"several fractions", []v1alpha2.CoreFractionValue{25, 50, 75}, true},
		{"two fractions", []v1alpha2.CoreFractionValue{50, 99}, true},
		{"no constraint", nil, true},
		{"single fraction", []v1alpha2.CoreFractionValue{50}, false},
		{"single fraction plus 100%", []v1alpha2.CoreFractionValue{50, 100}, false},
		{"only 100%", []v1alpha2.CoreFractionValue{100}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewCoreFractionValidator(coreFractionClient(t, tt.fractions...), newAutoscalerFeatureGate(t, true, true))

			if _, err := v.ValidateCreate(context.Background(), vmWithCoreFraction(v1alpha2.CoreFractionAuto)); (err == nil) != tt.valid {
				t.Errorf("ValidateCreate: got err %v, want valid=%v", err, tt.valid)
			}
			// An explicit fraction is none of this validator's business.
			if _, err := v.ValidateCreate(context.Background(), vmWithCoreFraction("50%")); err != nil {
				t.Errorf("explicit core fraction must be allowed, got %v", err)
			}
		})
	}
}

// TestCoreFractionMissingClass verifies that a VM referencing a class that does not
// exist yet is not rejected: it waits in Pending until the class shows up.
func TestCoreFractionMissingClass(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}
	v := NewCoreFractionValidator(fake.NewClientBuilder().WithScheme(scheme).Build(), newAutoscalerFeatureGate(t, true, true))

	if _, err := v.ValidateCreate(context.Background(), vmWithCoreFraction(v1alpha2.CoreFractionAuto)); err != nil {
		t.Errorf("a VM whose class does not exist yet must be allowed, got %v", err)
	}
}

// TestCoreFractionRatchet verifies that updates to a VM already on "Auto" are allowed
// even with the features disabled, while the transition into "Auto" stays gated.
func TestCoreFractionRatchet(t *testing.T) {
	gate := newAutoscalerFeatureGate(t, false, false)
	v := NewCoreFractionValidator(coreFractionClient(t, 25, 50), gate)
	auto := vmWithCoreFraction(v1alpha2.CoreFractionAuto)

	if _, err := v.ValidateUpdate(context.Background(), auto, auto); err != nil {
		t.Errorf("update of an existing Auto VM must be allowed with the features disabled, got %v", err)
	}

	if _, err := v.ValidateUpdate(context.Background(), vmWithCoreFraction("50%"), auto); err == nil {
		t.Error("transition to Auto with the features disabled must be rejected")
	}
}

// TestCoreFractionSizingPolicyRatchet verifies that narrowing the class under a VM that
// already runs on "Auto" does not block its updates.
func TestCoreFractionSizingPolicyRatchet(t *testing.T) {
	v := NewCoreFractionValidator(coreFractionClient(t, 50), newAutoscalerFeatureGate(t, true, true))
	auto := vmWithCoreFraction(v1alpha2.CoreFractionAuto)

	if _, err := v.ValidateUpdate(context.Background(), auto, auto); err != nil {
		t.Errorf("update of an existing Auto VM must be allowed with a single-step policy, got %v", err)
	}
}
