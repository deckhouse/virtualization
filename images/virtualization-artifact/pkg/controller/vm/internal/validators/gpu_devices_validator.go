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

	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type GPUDevicesValidator struct {
	client      client.Client
	featureGate featuregate.FeatureGate
}

func NewGPUDevicesValidator(client client.Client, featureGate featuregate.FeatureGate) *GPUDevicesValidator {
	return &GPUDevicesValidator{client: client, featureGate: featureGate}
}

func (v *GPUDevicesValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validateGPUDevices(ctx, vm)
}

func (v *GPUDevicesValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	// The feature gate and DeviceClass existence are checked only when GPU devices
	// are introduced or changed. Unchanged GPU devices must not block unrelated
	// updates (or removal) of a VM created while the gate was enabled and later
	// disabled, or whose DeviceClass was removed out of band. Order is ignored
	// to match the vmchange comparator, which treats reordering as no change.
	if reflect.DeepEqual(kvbuilder.SortGPUDevices(oldVM.Spec.GPUDevices), kvbuilder.SortGPUDevices(newVM.Spec.GPUDevices)) {
		return nil, nil
	}
	return nil, v.validateGPUDevices(ctx, newVM)
}

func (v *GPUDevicesValidator) validateGPUDevices(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	if len(vm.Spec.GPUDevices) == 0 {
		return nil
	}

	if !v.featureGate.Enabled(featuregates.GPU) {
		return fmt.Errorf("GPU device attachment requires the GPU feature gate")
	}

	// A nil client means template validation (e.g. a VirtualMachinePool template):
	// GPUClass readiness is verified when the actual replica VM is created, so that
	// a pool may be defined before the GPU provider and its classes exist.
	if v.client == nil {
		return nil
	}

	// The GPU module creates a DeviceClass named exactly after the GPUClass, so an
	// existing DeviceClass with that name means the GPUClass is ready for allocation.
	for _, device := range vm.Spec.GPUDevices {
		deviceClass := &resourcev1.DeviceClass{}
		err := v.client.Get(ctx, types.NamespacedName{Name: device.GPUClassName}, deviceClass)
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("GPU device %q references GPUClass %q that does not exist or is not ready", device.Name, device.GPUClassName)
		}
		if err != nil {
			return fmt.Errorf("failed to resolve GPUClass %q for GPU device %q: %w", device.GPUClassName, device.Name, err)
		}
	}

	return nil
}
