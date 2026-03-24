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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
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

	if err := v.validateNetworkIDsUnchanged(oldVM.Spec.Networks, newNetworksSpec, newVM.Status.Phase); err != nil {
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
	idsSet := make(map[int]struct{})
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

		if err := v.validateNetworkIDUniqueness(network, idsSet); err != nil {
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

func (v *NetworksValidator) validateNetworkIDUniqueness(network v1alpha2.NetworksSpec, idsSet map[int]struct{}) error {
	id := ptr.Deref(network.ID, 0)
	if id == 0 {
		return nil
	}
	if _, exists := idsSet[id]; exists {
		networkIdentifier := v.getNetworkIdentifier(network)
		return fmt.Errorf("network id %d is duplicated for network %s", id, networkIdentifier)
	}
	idsSet[id] = struct{}{}
	return nil
}

func (v *NetworksValidator) validateNetworkIDsUnchanged(oldNetworksSpec, newNetworksSpec []v1alpha2.NetworksSpec, phase v1alpha2.MachinePhase) error {
	oldNetworksMap := v.buildNetworksMap(oldNetworksSpec)
	newNetworksMap := v.buildNetworksMap(newNetworksSpec)

	for key, oldNetwork := range oldNetworksMap {
		newNetwork, exists := newNetworksMap[key]
		if !exists {
			continue
		}

		if ptr.Deref(oldNetwork.ID, 0) == ptr.Deref(newNetwork.ID, 0) {
			continue
		}

		if oldNetwork.ID == nil {
			continue
		}

		if phase != v1alpha2.MachineStopped {
			networkIdentifier := v.getNetworkIdentifier(oldNetwork)
			return fmt.Errorf("cannot change network ID for network '%s' while VM is in phase '%s' (only allowed when VM is stopped)",
				networkIdentifier, phase)
		}
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
	if network.ID == nil {
		return nil
	}

	id := *network.ID
	if id < 1 || id > commonnetwork.MaxID {
		networkIdentifier := v.getNetworkIdentifier(network)
		return fmt.Errorf("network id must be between 1 and %d for network %s, got %d", commonnetwork.MaxID, networkIdentifier, id)
	}

	if network.Type == v1alpha2.NetworksTypeMain && id != 1 {
		return fmt.Errorf("network id for network %s must be 1, got %d", network.Type, id)
	}

	return nil
}

func (v *NetworksValidator) getNetworkIdentifier(network v1alpha2.NetworksSpec) string {
	if network.Type == v1alpha2.NetworksTypeMain {
		return network.Type
	}
	return fmt.Sprintf("%s/%s", network.Type, network.Name)
}
