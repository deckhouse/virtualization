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
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type DefaultCoreFractionValidator struct{}

func NewDefaultCoreFractionValidator() *DefaultCoreFractionValidator {
	return &DefaultCoreFractionValidator{}
}

func (v *DefaultCoreFractionValidator) ValidateCreate(_ context.Context, vmclass *v1alpha2.VirtualMachineClass) (admission.Warnings, error) {
	return nil, v.validate(vmclass)
}

func (v *DefaultCoreFractionValidator) ValidateUpdate(_ context.Context, _, newVMClass *v1alpha2.VirtualMachineClass) (admission.Warnings, error) {
	return nil, v.validate(newVMClass)
}

func (v *DefaultCoreFractionValidator) validate(vmclass *v1alpha2.VirtualMachineClass) error {
	for i, policy := range vmclass.Spec.SizingPolicies {
		if policy.DefaultCoreFraction == nil {
			continue
		}

		if len(policy.CoreFractions) == 0 {
			continue
		}

		if !slices.Contains(policy.CoreFractions, *policy.DefaultCoreFraction) {
			return fmt.Errorf("vmclass %s sizingPolicy[%d]: defaultCoreFraction %d%% is not in the allowed coreFractions list %s",
				vmclass.Name, i, *policy.DefaultCoreFraction, sizingpolicy.FormatCoreFractionValues(policy.CoreFractions))
		}
	}

	return nil
}
