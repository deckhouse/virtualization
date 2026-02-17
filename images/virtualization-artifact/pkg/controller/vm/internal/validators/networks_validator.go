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

const (
	maxNetworkID = 16*1024 - 1 // 16383
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

	if !isSingleMainNet(networksSpec) && !v.featureGate.Enabled(featuregates.SDN) {
		return nil, fmt.Errorf("network configuration requires SDN to be enabled")
	}

	return v.validateNetworksSpec(networksSpec)
}

func (v *NetworksValidator) ValidateUpdate(_ context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	newNetworksSpec := newVM.Spec.Networks
	if len(newNetworksSpec) == 0 {
		return nil, nil
	}

	if !isSingleMainNet(newNetworksSpec) && !v.featureGate.Enabled(featuregates.SDN) {
		return nil, fmt.Errorf("network configuration requires SDN to be enabled")
	}

	if err := v.validateNetworkIDsUnchanged(oldVM.Spec.Networks, newNetworksSpec); err != nil {
		return nil, err
	}

	isChanged := !equality.Semantic.DeepEqual(newNetworksSpec, oldVM.Spec.Networks)
	if isChanged {
		return v.validateNetworksSpec(newNetworksSpec)
	}
	return nil, nil
}

func isSingleMainNet(networks []v1alpha2.NetworksSpec) bool {
	return len(networks) == 1 && networks[0].Type == v1alpha2.NetworksTypeMain
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

		if err := v.validateNetworkID(network); err != nil {
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

func (v *NetworksValidator) validateNetworkIDsUnchanged(oldNetworksSpec, newNetworksSpec []v1alpha2.NetworksSpec) error {
	oldNetworksMap := v.buildNetworksMap(oldNetworksSpec)
	newNetworksMap := v.buildNetworksMap(newNetworksSpec)

	for key, oldNetwork := range oldNetworksMap {
		newNetwork, exists := newNetworksMap[key]
		if !exists {
			continue
		}

		if oldNetwork.ID == newNetwork.ID {
			continue
		}

		if oldNetwork.ID == 0 && newNetwork.ID > 0 && newNetwork.ID <= maxNetworkID {
			continue
		}

		networkIdentifier := v.getNetworkIdentifier(oldNetwork)
		return fmt.Errorf("network id cannot be changed for network %s", networkIdentifier)
	}

	return nil
}

func (v *NetworksValidator) buildNetworksMap(networksSpec []v1alpha2.NetworksSpec) map[string]v1alpha2.NetworksSpec {
	networksMap := make(map[string]v1alpha2.NetworksSpec)
	for _, network := range networksSpec {
		key := v.getNetworkIdentifier(network)
		networksMap[key] = network
	}
	return networksMap
}

func (v *NetworksValidator) validateNetworkID(network v1alpha2.NetworksSpec) error {
	if network.ID == 0 {
		return nil
	}

	if network.ID < 1 || network.ID > maxNetworkID {
		networkIdentifier := v.getNetworkIdentifier(network)
		return fmt.Errorf("network id must be between 1 and %d for network %s, got %d", maxNetworkID, networkIdentifier, network.ID)
	}

	return nil
}

func (v *NetworksValidator) getNetworkIdentifier(network v1alpha2.NetworksSpec) string {
	if network.Type == v1alpha2.NetworksTypeMain {
		return network.Type
	}
	return fmt.Sprintf("%s/%s", network.Type, network.Name)
}
