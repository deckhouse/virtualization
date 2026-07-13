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

	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// CoreFractionValidator guards the automatic CPU core fraction
// (spec.cpu.coreFraction: "auto"). It may only be used when the machinery that
// drives it is available: the VerticalVirtualMachineAutoscaler feature (which
// itself requires the VerticalPodAutoscaler CRD and a supported edition) and the
// in-place CPU/memory resize feature (without it every autoscaling step would force
// a restart). The controller gates on the same features, so rejecting "auto" here
// keeps a VM from opting into a mode nothing would act on.
type CoreFractionValidator struct {
	featureGate featuregate.FeatureGate
}

func NewCoreFractionValidator(featureGate featuregate.FeatureGate) *CoreFractionValidator {
	return &CoreFractionValidator{featureGate: featureGate}
}

func (v *CoreFractionValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validate(vm)
}

func (v *CoreFractionValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validate(newVM)
}

func (v *CoreFractionValidator) validate(vm *v1alpha2.VirtualMachine) error {
	if vm.Spec.CPU.CoreFraction != v1alpha2.CoreFractionAuto {
		return nil
	}

	if !v.featureGate.Enabled(featuregates.VerticalVirtualMachineAutoscaler) {
		return fmt.Errorf("automatic CPU core fraction (spec.cpu.coreFraction: %q) is unavailable: vertical VirtualMachine autoscaling is disabled; it requires the VerticalPodAutoscaler CRD to be installed and a supported module edition", v1alpha2.CoreFractionAuto)
	}

	if !v.featureGate.Enabled(featuregates.HotplugCPUAndMemoryWithInPlaceResize) {
		return fmt.Errorf("automatic CPU core fraction (spec.cpu.coreFraction: %q) requires the in-place CPU and memory resize feature to be enabled", v1alpha2.CoreFractionAuto)
	}

	return nil
}
