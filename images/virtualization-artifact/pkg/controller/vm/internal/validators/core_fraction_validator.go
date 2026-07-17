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
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// CoreFractionValidator guards the automatic CPU core fraction
// (spec.cpu.coreFraction: "Auto"). It may only be used when the machinery that
// drives it is available: the VerticalVirtualMachineAutoscaler feature (which
// itself requires the vertical-pod-autoscaler module and a supported edition) and the
// in-place CPU/memory resize feature (without it every autoscaling step would force
// a restart). The controller gates on the same features, so rejecting "Auto" here
// keeps a VM from opting into a mode nothing would act on. The same goes for a sizing
// policy that leaves the autoscaler a single core fraction to pick from.
type CoreFractionValidator struct {
	client      client.Client
	featureGate featuregate.FeatureGate
}

func NewCoreFractionValidator(client client.Client, featureGate featuregate.FeatureGate) *CoreFractionValidator {
	return &CoreFractionValidator{client: client, featureGate: featureGate}
}

func (v *CoreFractionValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validate(ctx, nil, vm)
}

func (v *CoreFractionValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.validate(ctx, oldVM, newVM)
}

func (v *CoreFractionValidator) validate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) error {
	if newVM.Spec.CPU.CoreFraction != v1alpha2.CoreFractionAuto {
		return nil
	}

	// Ratchet: only guard the transition into "Auto". A VM already on "Auto" (e.g. the
	// features were disabled after it opted in) must stay editable, so unrelated updates —
	// memory, disks, restore flows — are not blocked by these gate checks.
	if oldVM != nil && oldVM.Spec.CPU.CoreFraction == v1alpha2.CoreFractionAuto {
		return nil
	}

	// The value may not be the user's own: a VirtualMachineClass with
	// `defaultCoreFraction: Auto` puts it on every VM created without an explicit core
	// fraction, so both messages point at where to set another value.
	if !v.featureGate.Enabled(featuregates.VerticalVirtualMachineAutoscaler) {
		return fmt.Errorf("automatic CPU core fraction (spec.cpu.coreFraction: %q, set explicitly or inherited from the VirtualMachineClass default) is unavailable: vertical VirtualMachine autoscaling is disabled; it requires the vertical-pod-autoscaler module to be enabled and a supported module edition; set an explicit core fraction (spec.cpu.coreFraction) instead", v1alpha2.CoreFractionAuto)
	}

	if !v.featureGate.Enabled(featuregates.HotplugCPUAndMemoryWithInPlaceResize) {
		return fmt.Errorf("automatic CPU core fraction (spec.cpu.coreFraction: %q, set explicitly or inherited from the VirtualMachineClass default) requires the in-place CPU and memory resize feature to be enabled; set an explicit core fraction (spec.cpu.coreFraction) instead", v1alpha2.CoreFractionAuto)
	}

	return v.validateSizingPolicy(ctx, newVM)
}

// validateSizingPolicy rejects "Auto" when the sizing policy of the chosen
// VirtualMachineClass leaves nothing to choose between. Autoscaling only ever picks
// core fractions the policy lists, and 100% is not one of them (it makes the pod
// Guaranteed), so a policy holding a single usable value would pin the VM to it forever.
func (v *CoreFractionValidator) validateSizingPolicy(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	class := &v1alpha2.VirtualMachineClass{}
	if err := v.client.Get(ctx, types.NamespacedName{Name: vm.Spec.VirtualMachineClassName}, class); err != nil {
		if k8serrors.IsNotFound(err) {
			// The class may be created later: the VM waits in Pending until then, and the
			// autoscaler picks the policy up once it exists.
			return nil
		}
		return fmt.Errorf("get VirtualMachineClass %q: %w", vm.Spec.VirtualMachineClassName, err)
	}

	policy := sizingpolicy.MatchSizingPolicy(class, vm.Spec.CPU.Cores)
	if policy == nil || sizingpolicy.CanAutoscaleCoreFraction(policy) {
		return nil
	}

	steps := sizingpolicy.AutoCoreFractionSteps(policy)
	allowed := "no core fraction below 100%"
	if len(steps) > 0 {
		allowed = fmt.Sprintf("a single core fraction below 100%% (%s)", strings.Join(sizingpolicy.FormatCoreFractionValues(steps), ", "))
	}

	return fmt.Errorf("automatic CPU core fraction (spec.cpu.coreFraction: %q, set explicitly or inherited from the VirtualMachineClass default) cannot be used with the VirtualMachineClass %q: its sizing policy for %d cores leaves %s, so there is nothing to scale between (100%% is never used automatically: it would make the virtual machine impossible to resize without a reboot); set an explicit core fraction (spec.cpu.coreFraction) instead",
		v1alpha2.CoreFractionAuto, class.Name, vm.Spec.CPU.Cores, allowed)
}
