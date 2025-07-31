/*
Copyright 2025 Flant JSC

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

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NetworksValidator struct{}

func NewNetworksValidator() *NetworksValidator {
	return &NetworksValidator{}
}

func (v *NetworksValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(vm)
}

func (v *NetworksValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(newVM)
}

func (v *NetworksValidator) Validate(vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	networksSpec := vm.Spec.Networks

	if len(networksSpec) == 0 {
		return nil, nil
	}

	if networksSpec[0].Type != v1alpha2.NetworksTypeMain {
		return nil, fmt.Errorf("first network in the list must be of type '%s'", v1alpha2.NetworksTypeMain)
	}
	if networksSpec[0].Name != "" {
		return nil, fmt.Errorf("network with type '%s' should not have a name", v1alpha2.NetworksTypeMain)
	}

	for i, network := range networksSpec {
		if network.Type == v1alpha2.NetworksTypeMain {
			continue
		}

		if network.Name == "" {
			return nil, fmt.Errorf("network at index %d with type '%s' must have a non-empty name", i, network.Type)
		}
	}

	return nil, nil
}
