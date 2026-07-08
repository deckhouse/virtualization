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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/featuregate"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestGPUDevicesValidatorValidateCreate(t *testing.T) {
	tests := []struct {
		name           string
		featureEnabled bool
		wantErrorPart  string
	}{
		{
			name:           "should reject GPU devices when feature is disabled",
			featureEnabled: false,
			wantErrorPart:  "GPU feature gate",
		},
		{
			name:           "should accept GPU devices when feature is enabled",
			featureEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := newVirtualMachineWithGPU("vm-current", []v1alpha2.GPUDeviceSpec{{Name: "gpu0", DeviceClassName: "nvidia-h100"}})
			validator := NewGPUDevicesValidator(newGPUFeatureGate(t, tt.featureEnabled))

			_, err := validator.ValidateCreate(t.Context(), vm)

			if tt.wantErrorPart == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrorPart) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErrorPart, err)
			}
		})
	}
}

func newVirtualMachineWithGPU(name string, gpuDevices []v1alpha2.GPUDeviceSpec) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       v1alpha2.VirtualMachineSpec{GPUDevices: gpuDevices},
	}
}

func newGPUFeatureGate(t *testing.T, enabled bool) featuregate.FeatureGate {
	t.Helper()

	gate, setFromMap, err := featuregates.NewUnlocked()
	if err != nil {
		t.Fatalf("failed to create feature gate: %v", err)
	}

	if err = setFromMap(map[string]bool{string(featuregates.GPU): enabled}); err != nil {
		t.Fatalf("failed to set GPU feature gate: %v", err)
	}

	return gate
}
