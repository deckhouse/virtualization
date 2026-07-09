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
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestGPUDevicesValidatorValidateCreate(t *testing.T) {
	tests := []struct {
		name           string
		featureEnabled bool
		deviceClasses  []string
		gpuClass       string
		wantErrorPart  string
	}{
		{
			name:           "should reject GPU devices when feature is disabled",
			featureEnabled: false,
			gpuClass:       "nvidia-h100",
			wantErrorPart:  "GPU feature gate",
		},
		{
			name:           "should accept GPU devices when feature is enabled and DeviceClass exists",
			featureEnabled: true,
			deviceClasses:  []string{"nvidia-h100"},
			gpuClass:       "nvidia-h100",
		},
		{
			name:           "should reject GPU devices when DeviceClass does not exist",
			featureEnabled: true,
			gpuClass:       "nvidia-h100",
			wantErrorPart:  "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := newVirtualMachineWithGPU("vm-current", []v1alpha2.GPUDeviceSpec{{Name: "gpu0", DeviceClassName: tt.gpuClass}})
			validator := NewGPUDevicesValidator(newValidatorClient(t, tt.deviceClasses...), newGPUFeatureGate(t, tt.featureEnabled))

			_, err := validator.ValidateCreate(t.Context(), vm)

			assertValidationError(t, err, tt.wantErrorPart)
		})
	}
}

func TestGPUDevicesValidatorValidateUpdate(t *testing.T) {
	gpu := func(class string) []v1alpha2.GPUDeviceSpec {
		return []v1alpha2.GPUDeviceSpec{{Name: "gpu0", DeviceClassName: class}}
	}

	tests := []struct {
		name           string
		featureEnabled bool
		deviceClasses  []string
		oldGPU         []v1alpha2.GPUDeviceSpec
		newGPU         []v1alpha2.GPUDeviceSpec
		wantErrorPart  string
	}{
		{
			name:           "unchanged GPU devices are allowed when feature is disabled",
			featureEnabled: false,
			oldGPU:         gpu("nvidia-h100"),
			newGPU:         gpu("nvidia-h100"),
		},
		{
			name:           "removing GPU devices is allowed when feature is disabled",
			featureEnabled: false,
			oldGPU:         gpu("nvidia-h100"),
			newGPU:         nil,
		},
		{
			name:           "reordering GPU devices is allowed when feature is disabled",
			featureEnabled: false,
			oldGPU: []v1alpha2.GPUDeviceSpec{
				{Name: "gpu0", DeviceClassName: "nvidia-h100"},
				{Name: "gpu1", DeviceClassName: "nvidia-a100"},
			},
			newGPU: []v1alpha2.GPUDeviceSpec{
				{Name: "gpu1", DeviceClassName: "nvidia-a100"},
				{Name: "gpu0", DeviceClassName: "nvidia-h100"},
			},
		},
		{
			name:           "adding GPU devices is rejected when feature is disabled",
			featureEnabled: false,
			oldGPU:         nil,
			newGPU:         gpu("nvidia-h100"),
			wantErrorPart:  "GPU feature gate",
		},
		{
			name:           "changing to an existing DeviceClass is allowed when feature is enabled",
			featureEnabled: true,
			deviceClasses:  []string{"nvidia-h100", "nvidia-a100"},
			oldGPU:         gpu("nvidia-h100"),
			newGPU:         gpu("nvidia-a100"),
		},
		{
			name:           "changing to a missing DeviceClass is rejected",
			featureEnabled: true,
			deviceClasses:  []string{"nvidia-h100"},
			oldGPU:         gpu("nvidia-h100"),
			newGPU:         gpu("nvidia-a100"),
			wantErrorPart:  "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldVM := newVirtualMachineWithGPU("vm-current", tt.oldGPU)
			newVM := newVirtualMachineWithGPU("vm-current", tt.newGPU)
			validator := NewGPUDevicesValidator(newValidatorClient(t, tt.deviceClasses...), newGPUFeatureGate(t, tt.featureEnabled))

			_, err := validator.ValidateUpdate(t.Context(), oldVM, newVM)

			assertValidationError(t, err, tt.wantErrorPart)
		})
	}
}

func TestGPUDevicesValidatorTemplateMode(t *testing.T) {
	// A nil client (template validation) enforces the feature gate but skips
	// DeviceClass existence.
	tests := []struct {
		name           string
		featureEnabled bool
		wantErrorPart  string
	}{
		{
			name:           "gate disabled is rejected in template mode",
			featureEnabled: false,
			wantErrorPart:  "GPU feature gate",
		},
		{
			name:           "missing DeviceClass is allowed in template mode",
			featureEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := newVirtualMachineWithGPU("vm-current", []v1alpha2.GPUDeviceSpec{{Name: "gpu0", DeviceClassName: "does-not-exist"}})
			validator := NewGPUDevicesValidator(nil, newGPUFeatureGate(t, tt.featureEnabled))

			_, err := validator.ValidateCreate(t.Context(), vm)

			assertValidationError(t, err, tt.wantErrorPart)
		})
	}
}

func assertValidationError(t *testing.T, err error, wantErrorPart string) {
	t.Helper()

	if wantErrorPart == "" {
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		return
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), wantErrorPart) {
		t.Fatalf("expected error containing %q, got %v", wantErrorPart, err)
	}
}

func newVirtualMachineWithGPU(name string, gpuDevices []v1alpha2.GPUDeviceSpec) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       v1alpha2.VirtualMachineSpec{GPUDevices: gpuDevices},
	}
}

func newValidatorClient(t *testing.T, deviceClasses ...string) client.Client {
	t.Helper()

	objs := make([]client.Object, 0, len(deviceClasses))
	for _, name := range deviceClasses {
		objs = append(objs, &resourcev1.DeviceClass{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}

	fakeClient, err := testutil.NewFakeClientWithObjects(objs...)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}
	return fakeClient
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
