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

package defaulter

import (
	"context"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

type CoreFractionDefaulter struct {
	client client.Client
}

func NewCoreFractionDefaulter(client client.Client) *CoreFractionDefaulter {
	return &CoreFractionDefaulter{
		client: client,
	}
}

func (d *CoreFractionDefaulter) Default(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	// Skip if coreFraction is already set.
	if vm.Spec.CPU.CoreFraction != "" {
		return nil
	}

	// Skip if vmClassName is not set (will be handled by validation later).
	if vm.Spec.VirtualMachineClassName == "" {
		return nil
	}

	// Get the VMClass.
	vmClass := &v1alpha3.VirtualMachineClass{}
	err := d.client.Get(ctx, types.NamespacedName{Name: vm.Spec.VirtualMachineClassName}, vmClass)
	if err != nil {
		return fmt.Errorf("failed to get VirtualMachineClass %q: %w", vm.Spec.VirtualMachineClassName, err)
	}

	// Find the matching sizing policy based on CPU cores.
	defaultCoreFraction, err := d.getDefaultCoreFraction(vm, vmClass)
	if err != nil {
		return err
	}
	vm.Spec.CPU.CoreFraction = defaultCoreFraction

	return nil
}

// getDefaultCoreFraction finds the default core fraction from the VMClass sizing policy
// that matches the VM's CPU cores count.
func (d *CoreFractionDefaulter) getDefaultCoreFraction(vm *v1alpha2.VirtualMachine, vmClass *v1alpha3.VirtualMachineClass) (string, error) {
	const defaultValue = "100%"

	if vmClass == nil || len(vmClass.Spec.SizingPolicies) == 0 {
		return defaultValue, nil
	}

	for _, sp := range vmClass.Spec.SizingPolicies {
		if sp.Cores == nil {
			continue
		}

		// Check if VM's cores fall within this policy's range.
		if vm.Spec.CPU.Cores >= sp.Cores.Min && vm.Spec.CPU.Cores <= sp.Cores.Max {
			switch {
			case sp.DefaultCoreFraction != nil:
				return string(*sp.DefaultCoreFraction), nil
			case len(sp.CoreFractions) > 0 && !slices.Contains(sp.CoreFractions, defaultValue):
				return "", fmt.Errorf(
					"the default value for core fraction is not defined. For the specified configuration \".spec.cpu.cores %d\", "+
						"the following core fractions are allowed: %v. Please specify the \".spec.core.coreFraction\" value and try again",
					vm.Spec.CPU.Cores,
					sp.CoreFractions,
				)
			default:
				return defaultValue, nil
			}
		}
	}

	return "", fmt.Errorf("the specified \".spec.cpu.cores %d\" value is not among the sizing policies allowed for the virtual machine", vm.Spec.CPU.Cores)
}
