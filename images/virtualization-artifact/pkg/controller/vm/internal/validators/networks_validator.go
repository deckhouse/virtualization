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

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NetworksValidator struct {
	featureGate featuregate.FeatureGate
}

func NewNetworksValidator(featureGate featuregate.FeatureGate) *NetworksValidator {
	return &NetworksValidator{
		featureGate: featureGate,
	}
}

func (v *NetworksValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	networksSpec := vm.Spec.Networks
	if len(networksSpec) == 0 {
		return nil, nil
	}

	if !v.featureGate.Enabled(featuregates.SDN) {
		return nil, fmt.Errorf("network configuration requires SDN to be enabled")
	}

	return v.validateNetworksSpec(networksSpec)
}

func (v *NetworksValidator) ValidateUpdate(_ context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	newNetworksSpec := newVM.Spec.Networks
	if len(newNetworksSpec) == 0 {
		return nil, nil
	}

	if !v.featureGate.Enabled(featuregates.SDN) {
		return nil, fmt.Errorf("network configuration requires SDN to be enabled")
	}

	isChanged := !equality.Semantic.DeepEqual(newNetworksSpec, oldVM.Spec.Networks)
	if isChanged {
		return v.validateNetworksSpec(newNetworksSpec)
	}
	return nil, nil
}

func (v *NetworksValidator) validateNetworksSpec(networksSpec []v1alpha2.NetworksSpec) (admission.Warnings, error) {
	namesSet := make(map[string]struct{})
	for i, network := range networksSpec {
		typ := network.Type
		name := network.Name

		if typ == v1alpha2.NetworksTypeMain && i > 0 {
			return nil, fmt.Errorf("first network in the list must be of type '%s'", v1alpha2.NetworksTypeMain)
		}

		if err := v.validateNetworkName(typ, name); err != nil {
			return nil, err
		}

		if err := v.validateNetworkUniqueness(typ, name, namesSet); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (v *NetworksValidator) validateNetworkName(networkType, networkName string) error {
	if networkType == v1alpha2.NetworksTypeMain {
		if networkName != "" {
			return fmt.Errorf("network with type '%s' should not have a name", v1alpha2.NetworksTypeMain)
		}
		return nil
	}

	if networkName == "" {
		return fmt.Errorf("network with type '%s' must have a non-empty name", networkType)
	}

	return nil
}

func (v *NetworksValidator) validateNetworkUniqueness(networkType, networkName string, namesSet map[string]struct{}) error {
	key := fmt.Sprintf("%s/%s", networkType, networkName)
	if _, exists := namesSet[key]; exists {
		if networkName != "" {
			return fmt.Errorf("network %s:%s is duplicated", networkType, networkName)
		}
		return fmt.Errorf("network %s is duplicated", networkType)
	}
	namesSet[key] = struct{}{}
	return nil
}
