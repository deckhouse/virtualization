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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
		gpuClasses     []string
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
			name:           "should accept GPU devices when feature is enabled and GPUClass is ready",
			featureEnabled: true,
			gpuClasses:     []string{"nvidia-h100"},
			gpuClass:       "nvidia-h100",
		},
		{
			name:           "should reject GPU devices when GPUClass is not ready",
			featureEnabled: true,
			gpuClass:       "nvidia-h100",
			wantErrorPart:  "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := newVirtualMachineWithGPU([]v1alpha2.GPUDeviceSpec{{GPUClassName: tt.gpuClass}})
			validator := NewGPUDevicesValidator(newValidatorClient(t, tt.gpuClasses...), newGPUFeatureGate(t, tt.featureEnabled))

			_, err := validator.ValidateCreate(t.Context(), vm)

			assertValidationError(t, err, tt.wantErrorPart)
		})
	}
}

func TestGPUDevicesValidatorValidateUpdate(t *testing.T) {
	gpu := func(class string) []v1alpha2.GPUDeviceSpec {
		return []v1alpha2.GPUDeviceSpec{{GPUClassName: class}}
	}

	tests := []struct {
		name           string
		featureEnabled bool
		gpuClasses     []string
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
				{GPUClassName: "nvidia-h100"},
				{GPUClassName: "nvidia-a100"},
			},
			newGPU: []v1alpha2.GPUDeviceSpec{
				{GPUClassName: "nvidia-a100"},
				{GPUClassName: "nvidia-h100"},
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
			name:           "changing to a ready GPUClass is allowed when feature is enabled",
			featureEnabled: true,
			gpuClasses:     []string{"nvidia-h100", "nvidia-a100"},
			oldGPU:         gpu("nvidia-h100"),
			newGPU:         gpu("nvidia-a100"),
		},
		{
			name:           "changing to an unready GPUClass is rejected",
			featureEnabled: true,
			gpuClasses:     []string{"nvidia-h100"},
			oldGPU:         gpu("nvidia-h100"),
			newGPU:         gpu("nvidia-a100"),
			wantErrorPart:  "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldVM := newVirtualMachineWithGPU(tt.oldGPU)
			newVM := newVirtualMachineWithGPU(tt.newGPU)
			validator := NewGPUDevicesValidator(newValidatorClient(t, tt.gpuClasses...), newGPUFeatureGate(t, tt.featureEnabled))

			_, err := validator.ValidateUpdate(t.Context(), oldVM, newVM)

			assertValidationError(t, err, tt.wantErrorPart)
		})
	}
}

func TestGPUDevicesValidatorTemplateMode(t *testing.T) {
	// A nil client (template validation) enforces the feature gate but skips
	// GPUClass existence.
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
			name:           "unready GPUClass is allowed in template mode",
			featureEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := newVirtualMachineWithGPU([]v1alpha2.GPUDeviceSpec{{GPUClassName: "does-not-exist"}})
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

func newVirtualMachineWithGPU(gpuDevices []v1alpha2.GPUDeviceSpec) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm-current", Namespace: "default"},
		Spec:       v1alpha2.VirtualMachineSpec{GPUDevices: gpuDevices},
	}
}

func newValidatorClient(t *testing.T, gpuClasses ...string) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(kvbuilder.GPUClassGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(kvbuilder.GPUClassGVK.GroupVersion().WithKind("GPUClassList"), &unstructured.UnstructuredList{})

	objs := make([]client.Object, 0, len(gpuClasses))
	for _, name := range gpuClasses {
		gpuClass := &unstructured.Unstructured{}
		gpuClass.SetGroupVersionKind(kvbuilder.GPUClassGVK)
		gpuClass.SetName(name)
		objs = append(objs, gpuClass)
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
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
