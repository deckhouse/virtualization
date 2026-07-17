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
	"strings"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type DefaultCoreFractionValidator struct {
	featureGate featuregate.FeatureGate
}

func NewDefaultCoreFractionValidator(featureGate featuregate.FeatureGate) *DefaultCoreFractionValidator {
	return &DefaultCoreFractionValidator{featureGate: featureGate}
}

func (v *DefaultCoreFractionValidator) ValidateCreate(_ context.Context, vmclass *v1alpha2.VirtualMachineClass) (admission.Warnings, error) {
	return nil, v.validate(nil, vmclass)
}

func (v *DefaultCoreFractionValidator) ValidateUpdate(_ context.Context, oldVMClass, newVMClass *v1alpha2.VirtualMachineClass) (admission.Warnings, error) {
	return nil, v.validate(oldVMClass, newVMClass)
}

func (v *DefaultCoreFractionValidator) validate(oldVMClass, newVMClass *v1alpha2.VirtualMachineClass) error {
	for i, policy := range newVMClass.Spec.SizingPolicies {
		if policy.DefaultCoreFraction == nil {
			continue
		}

		if policy.DefaultCoreFraction.Type == intstr.String {
			if err := v.validateAuto(oldVMClass, newVMClass, i, &policy, policy.DefaultCoreFraction.StrVal); err != nil {
				return err
			}
			continue
		}

		if len(policy.CoreFractions) == 0 {
			continue
		}

		fraction := v1alpha2.CoreFractionValue(policy.DefaultCoreFraction.IntValue())
		if !slices.Contains(policy.CoreFractions, fraction) {
			return fmt.Errorf("VirtualMachineClass %q: the default core fraction (spec.sizingPolicies[%d].defaultCoreFraction) %d%% is not among the allowed core fractions; set it to one of: %s",
				newVMClass.Name, i, fraction, strings.Join(sizingpolicy.FormatCoreFractionValues(policy.CoreFractions), ", "))
		}
	}

	return nil
}

// validateAuto guards the only allowed string value of defaultCoreFraction. `Auto` hands
// the core fraction over to the Vertical VirtualMachine Autoscaler, so it is deliberately
// not required to be one of the allowed coreFractions — it is a mode, not a share of a CPU
// core. It is only accepted when the machinery that drives it is available, and when the
// policy leaves it room to move: otherwise every VM defaulted to `Auto` by this class would
// be rejected by the VirtualMachine webhook, and the class would be unusable.
func (v *DefaultCoreFractionValidator) validateAuto(oldVMClass, newVMClass *v1alpha2.VirtualMachineClass, policyIndex int, policy *v1alpha2.SizingPolicy, value string) error {
	if value != string(v1alpha2.CoreFractionAuto) {
		return fmt.Errorf("VirtualMachineClass %q: the default core fraction (spec.sizingPolicies[%d].defaultCoreFraction) %q is invalid; set a percentage from 1%% to 100%%, or %q",
			newVMClass.Name, policyIndex, value, v1alpha2.CoreFractionAuto)
	}

	// Ratchet: only guard the transition into "Auto". A class that already uses it (e.g.
	// the features were disabled afterwards) must stay editable, so unrelated changes —
	// node selector, tolerations, memory limits — are not blocked by these gate checks.
	if hasAutoDefaultCoreFraction(oldVMClass) {
		return nil
	}

	if !v.featureGate.Enabled(featuregates.VerticalVirtualMachineAutoscaler) {
		return fmt.Errorf("VirtualMachineClass %q: the automatic default core fraction (spec.sizingPolicies[%d].defaultCoreFraction: %q) is unavailable: vertical VirtualMachine autoscaling is disabled; it requires the vertical-pod-autoscaler module to be enabled and a supported module edition",
			newVMClass.Name, policyIndex, v1alpha2.CoreFractionAuto)
	}

	if !v.featureGate.Enabled(featuregates.HotplugCPUAndMemoryWithInPlaceResize) {
		return fmt.Errorf("VirtualMachineClass %q: the automatic default core fraction (spec.sizingPolicies[%d].defaultCoreFraction: %q) requires the in-place CPU and memory resize feature to be enabled",
			newVMClass.Name, policyIndex, v1alpha2.CoreFractionAuto)
	}

	// Autoscaling only picks core fractions this policy allows, and 100% is never picked
	// automatically: requests would equal limits and the virtual machine could no longer be
	// resized without a reboot. A policy left with a single usable value would therefore pin
	// every VM of this class to it, which is what an explicit percentage is for.
	if !sizingpolicy.CanAutoscaleCoreFraction(policy) {
		steps := sizingpolicy.AutoCoreFractionSteps(policy)
		allowed := "no core fraction below 100%"
		if len(steps) > 0 {
			allowed = fmt.Sprintf("a single core fraction below 100%% (%s)", strings.Join(sizingpolicy.FormatCoreFractionValues(steps), ", "))
		}
		return fmt.Errorf("VirtualMachineClass %q: the automatic default core fraction (spec.sizingPolicies[%d].defaultCoreFraction: %q) needs at least %d core fractions to choose from, but the policy leaves %s; list more core fractions (spec.sizingPolicies[%d].coreFractions) or set an explicit default",
			newVMClass.Name, policyIndex, v1alpha2.CoreFractionAuto, sizingpolicy.MinAutoCoreFractionSteps, allowed, policyIndex)
	}

	return nil
}

func hasAutoDefaultCoreFraction(vmclass *v1alpha2.VirtualMachineClass) bool {
	if vmclass == nil {
		return false
	}
	for _, policy := range vmclass.Spec.SizingPolicies {
		if policy.DefaultCoreFraction == nil {
			continue
		}
		if policy.DefaultCoreFraction.Type == intstr.String && policy.DefaultCoreFraction.StrVal == string(v1alpha2.CoreFractionAuto) {
			return true
		}
	}
	return false
}
