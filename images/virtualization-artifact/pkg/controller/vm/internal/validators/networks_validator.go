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
	if newVM == nil || !newVM.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

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
	if networksSpec[0].Type != v1alpha2.NetworksTypeMain {
		return nil, fmt.Errorf("first network in the list must be of type '%s'", v1alpha2.NetworksTypeMain)
	}
	if networksSpec[0].Name != "" {
		return nil, fmt.Errorf("network with type '%s' should not have a name", v1alpha2.NetworksTypeMain)
	}

	namesSet := make(map[string]struct{})

	for i, network := range networksSpec {
		if network.Type == v1alpha2.NetworksTypeMain {
			if i > 0 {
				return nil, fmt.Errorf("only one network of type '%s' is allowed", v1alpha2.NetworksTypeMain)
			}
			continue
		}
		if network.Name == "" {
			return nil, fmt.Errorf("network at index %d with type '%s' must have a non-empty name", i, network.Type)
		}

		if _, exists := namesSet[network.Name]; exists {
			return nil, fmt.Errorf("network name '%s' is duplicated", network.Name)
		}
		namesSet[network.Name] = struct{}{}
	}

	return nil, nil
}
