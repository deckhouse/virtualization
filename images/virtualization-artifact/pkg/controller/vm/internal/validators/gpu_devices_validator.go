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
	"fmt"
	"reflect"

	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type GPUDevicesValidator struct {
	featureGate featuregate.FeatureGate
}

func NewGPUDevicesValidator(featureGate featuregate.FeatureGate) *GPUDevicesValidator {
	return &GPUDevicesValidator{featureGate: featureGate}
}

func (v *GPUDevicesValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validateGPUDevices(vm)
}

func (v *GPUDevicesValidator) ValidateUpdate(_ context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	// The feature gate is required only when GPU devices are introduced or changed.
	// Unchanged GPU devices must not block unrelated updates (or removal) of a VM
	// created while the gate was enabled and later disabled.
	if reflect.DeepEqual(oldVM.Spec.GPUDevices, newVM.Spec.GPUDevices) {
		return nil, nil
	}
	return nil, v.validateGPUDevices(newVM)
}

func (v *GPUDevicesValidator) validateGPUDevices(vm *v1alpha2.VirtualMachine) error {
	if len(vm.Spec.GPUDevices) == 0 {
		return nil
	}

	if !v.featureGate.Enabled(featuregates.GPU) {
		return fmt.Errorf("GPU device attachment requires the GPU feature gate")
	}

	return nil
}
