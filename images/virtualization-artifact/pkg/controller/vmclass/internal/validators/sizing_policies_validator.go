/*
Copyright 2024 Flant JSC

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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SizingPoliciesValidator struct {
	client client.Client
}

func NewSizingPoliciesValidator(client client.Client) *SizingPoliciesValidator {
	return &SizingPoliciesValidator{client: client}
}

func (v *SizingPoliciesValidator) ValidateCreate(_ context.Context, vmclass *v1alpha2.VirtualMachineClass) (admission.Warnings, error) {
	if err := validateCoreFractions(vmclass); err != nil {
		return nil, err
	}

	if !HasValidCores(&vmclass.Spec) {
		return nil, fmt.Errorf("vmclass %s has sizing policies but none of them specify cores", vmclass.Name)
	}

	if HasCPUSizePoliciesCrosses(&vmclass.Spec) {
		return nil, fmt.Errorf("vmclass %s has size policy cpu crosses", vmclass.Name)
	}

	return nil, nil
}

func (v *SizingPoliciesValidator) ValidateUpdate(_ context.Context, _, newVMClass *v1alpha2.VirtualMachineClass) (admission.Warnings, error) {
	if err := validateCoreFractions(newVMClass); err != nil {
		return nil, err
	}

	if HasCPUSizePoliciesCrosses(&newVMClass.Spec) {
		return nil, fmt.Errorf("vmclass %s has size policy cpu crosses", newVMClass.Name)
	}

	return nil, nil
}

func HasCPUSizePoliciesCrosses(vmclass *v1alpha2.VirtualMachineClassSpec) bool {
	usedPairs := make(map[[2]int]struct{})

	for i, policy1 := range vmclass.SizingPolicies {
		for j, policy2 := range vmclass.SizingPolicies {
			if i == j {
				continue
			}
			_, ok := usedPairs[[2]int{i, j}]
			if ok {
				continue
			}

			if policy1.Cores == nil || policy2.Cores == nil {
				continue
			}

			if policy1.Cores.Min >= policy2.Cores.Min && policy1.Cores.Min <= policy2.Cores.Max {
				return true
			}

			if policy1.Cores.Max >= policy2.Cores.Min && policy1.Cores.Max <= policy2.Cores.Max {
				return true
			}

			usedPairs[[2]int{i, j}] = struct{}{}
			usedPairs[[2]int{j, i}] = struct{}{}
		}
	}

	return false
}

func HasValidCores(vmclass *v1alpha2.VirtualMachineClassSpec) bool {
	if len(vmclass.SizingPolicies) == 0 {
		return true
	}

	for _, policy := range vmclass.SizingPolicies {
		if policy.Cores == nil {
			return false
		}
	}
	return true
}

func validateCoreFractions(vmclass *v1alpha2.VirtualMachineClass) error {
	for i, policy := range vmclass.Spec.SizingPolicies {
		for j, coreFraction := range policy.CoreFractions {
			if coreFraction < 1 || coreFraction > 100 {
				return fmt.Errorf("spec.sizingPolicies[%d].coreFractions[%d]: coreFraction must be between 1 and 100, got %d", i, j, coreFraction)
			}
		}
	}
	return nil
}
