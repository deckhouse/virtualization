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

	"k8s.io/component-base/featuregate"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

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
			CPU: v1alpha2.CPUSpec{Cores: 2, CoreFraction: coreFraction},
		},
	}
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
			v := NewCoreFractionValidator(gate)
			vm := vmWithCoreFraction(tt.coreFraction)

			if _, err := v.ValidateCreate(context.Background(), vm); (err == nil) != tt.valid {
				t.Errorf("ValidateCreate: got err %v, want valid=%v", err, tt.valid)
			}
			if _, err := v.ValidateUpdate(context.Background(), vm, vm); (err == nil) != tt.valid {
				t.Errorf("ValidateUpdate: got err %v, want valid=%v", err, tt.valid)
			}
		})
	}
}
