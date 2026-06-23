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

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestGPUDevicesValidatorValidateCreate(t *testing.T) {
	tests := []struct {
		name           string
		featureEnabled bool
		objects        []client.Object
		wantErrorPart  string
	}{
		{
			name:           "should reject GPU devices when feature is disabled",
			featureEnabled: false,
			objects:        []client.Object{newGPUDeviceClass()},
			wantErrorPart:  "GPU feature gate",
		},
		{
			name:           "should reject GPU devices when DeviceClass is missing",
			featureEnabled: true,
			wantErrorPart:  "DeviceClass",
		},
		{
			name:           "should accept GPU devices when feature and DeviceClass are available",
			featureEnabled: true,
			objects:        []client.Object{newGPUDeviceClass()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := newVirtualMachineWithGPU("vm-current", []v1alpha2.GPUDeviceSpec{{Name: "gpu0", Model: "NVIDIA H100"}})
			validator := NewGPUDevicesValidator(newFakeClientWithResourceObjects(t, tt.objects...), newGPUFeatureGate(t, tt.featureEnabled))

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

func newGPUDeviceClass() *resourcev1.DeviceClass {
	return &resourcev1.DeviceClass{ObjectMeta: metav1.ObjectMeta{Name: kvbuilder.GPUDeviceClassName}}
}

func newFakeClientWithResourceObjects(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add virtualization API scheme: %v", err)
	}
	if err := resourcev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add resource API scheme: %v", err)
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
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
