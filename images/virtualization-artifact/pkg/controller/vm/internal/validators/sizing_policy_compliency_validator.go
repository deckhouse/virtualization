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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type IClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
}

type SizingPolicyCompliencyValidator struct {
	client IClient
}

func NewSizingPolicyCompliencyValidator(client IClient) *SizingPolicyCompliencyValidator {
	return &SizingPolicyCompliencyValidator{client: client}
}

func (v *SizingPolicyCompliencyValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	err := v.CheckVMCompliedSizePolicy(ctx, vm)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *SizingPolicyCompliencyValidator) ValidateUpdate(ctx context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	err := v.CheckVMCompliedSizePolicy(ctx, newVM)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *SizingPolicyCompliencyValidator) CheckVMCompliedSizePolicy(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	vmclass := v1alpha2.VirtualMachineClass{}
	err := v.client.Get(ctx, types.NamespacedName{
		Name: vm.Spec.VirtualMachineClassName,
	}, &vmclass)
	if err != nil {
		return err
	}

	sizePolicy := getVMSizePolicy(vm, vmclass)
	if sizePolicy == nil {
		return fmt.Errorf(
			"virtual machine %s has no valid size policy in vm class %s",
			vm.Name, vm.Spec.VirtualMachineClassName,
		)
	}

	errorsArray := make([]error, 0)

	errorsArray = append(errorsArray, validateCoreFraction(vm, sizePolicy)...)
	errorsArray = append(errorsArray, validateVMMemory(vm, sizePolicy)...)
	errorsArray = append(errorsArray, validatePerCoreMemory(vm, sizePolicy)...)

	if len(errorsArray) > 0 {
		return fmt.Errorf("errors while size policy validate: %w", errors.Join(errorsArray...))
	}

	return nil
}

func getVMSizePolicy(vm *v1alpha2.VirtualMachine, vmclass v1alpha2.VirtualMachineClass) *v1alpha2.SizingPolicy {
	for _, sp := range vmclass.Spec.SizingPolicies {
		if vm.Spec.CPU.Cores >= sp.Cores.Min && vm.Spec.CPU.Cores <= sp.Cores.Max {
			return sp.DeepCopy()
		}
	}

	return nil
}

func validateCoreFraction(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) (errorsArray []error) {
	errorsArray = make([]error, 0)

	if sp.CoreFractions == nil {
		return
	}

	fractionStr := strings.ReplaceAll(vm.Spec.CPU.CoreFraction, "%", "")
	fraction, err := strconv.Atoi(fractionStr)
	if err != nil {
		errorsArray = append(errorsArray, fmt.Errorf("cpu fraction value is invalid"))
		return
	}

	hasInSizePolicyFractions := false
	for _, spFraction := range sp.CoreFractions {
		if fraction == int(spFraction) {
			hasInSizePolicyFractions = true
		}
	}

	if !hasInSizePolicyFractions {
		errorsArray = append(errorsArray, fmt.Errorf("vm core fraction incorrect"))
	}

	return
}

func validateVMMemory(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) (errorsArray []error) {
	errorsArray = make([]error, 0)

	if sp.Memory == nil {
		return
	}

	if sp.Memory.Max.String() == "0" { // has not nil, must check like this
		return
	}

	if vm.Spec.Memory.Size.Cmp(sp.Memory.Min) == -1 {
		errorsArray = append(errorsArray, fmt.Errorf(
			"requested VM memory (%s) lesser than valid minimum, available range [%s, %s]",
			vm.Spec.Memory.Size.String(),
			sp.Memory.Min.String(),
			sp.Memory.Max.String(),
		))
	}

	if vm.Spec.Memory.Size.Cmp(sp.Memory.Max) == 1 {
		errorsArray = append(errorsArray, fmt.Errorf(
			"requested VM memory (%s) greater than valid maximum, available range [%s, %s]",
			vm.Spec.Memory.Size.String(),
			sp.Memory.Min.String(),
			sp.Memory.Max.String(),
		))
	}

	if sp.Memory.Step.String() != "0" {
		err := checkInGrid(vm.Spec.Memory.Size, sp.Memory.Min, sp.Memory.Max, sp.Memory.Step, "VM memory")
		if err != nil {
			errorsArray = append(errorsArray, err)
		}
	}

	return
}

func validatePerCoreMemory(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) (errorsArray []error) {
	errorsArray = make([]error, 0)

	if sp.Memory == nil {
		return
	}

	if sp.Memory.PerCore.Max.String() == "0" { // has not nil, must check like this
		return
	}

	// not have a default dividing :(
	// dirty, I know
	// wash your hands after read this
	vmMemoryValueInt64, _ := vm.Spec.Memory.Size.AsInt64()
	vmPerCore := vmMemoryValueInt64 / int64(vm.Spec.CPU.Cores)
	perCoreMemory := resource.MustParse(fmt.Sprintf("%dKi", vmPerCore/1024))

	if perCoreMemory.Cmp(sp.Memory.PerCore.Min) == -1 {
		errorsArray = append(errorsArray, fmt.Errorf(
			"requested VM per core memory (%s) lesser than valid minimum, available range [%s, %s]",
			perCoreMemory.String(),
			sp.Memory.PerCore.Min.String(),
			sp.Memory.PerCore.Max.String(),
		))
	}

	if perCoreMemory.Cmp(sp.Memory.PerCore.Max) == 1 {
		errorsArray = append(errorsArray, fmt.Errorf(
			"requested VM per core memory (%s) greater than valid maximum, available range [%s, %s]",
			perCoreMemory.String(),
			sp.Memory.PerCore.Min.String(),
			sp.Memory.PerCore.Max.String(),
		))
	}

	if sp.Memory.Step.String() != "0" {
		err := checkInGrid(perCoreMemory, sp.Memory.PerCore.Min, sp.Memory.PerCore.Max, sp.Memory.Step, "VM per core memory")
		if err != nil {
			errorsArray = append(errorsArray, err)
		}
	}

	return
}

func checkInGrid(value, min, max, step resource.Quantity, source string) (err error) {
	err = nil
	grid := generateValidGrid(min, max, step)

	for i := 0; i < len(grid)-1; i++ {
		cmpLeftResult := value.Cmp(grid[i])
		cmpRightResult := value.Cmp(grid[i+1])

		if cmpLeftResult == 0 || cmpRightResult == 0 {
			return
		} else if cmpLeftResult == 1 && cmpRightResult == -1 {
			err = fmt.Errorf(
				"requested %s not in available values grid, nearest valid values [%s, %s]",
				source,
				grid[i].String(),
				grid[i+1].String(),
			)
			return
		}
	}

	return
}

func generateValidGrid(min, max, step resource.Quantity) []resource.Quantity {
	grid := make([]resource.Quantity, 0)

	for val := min; val.Cmp(max) == -1; val.Add(step) {
		grid = append(grid, val)
	}

	grid = append(grid, max)

	return grid
}
