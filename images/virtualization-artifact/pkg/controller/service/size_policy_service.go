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

package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	lesser  = -1
	equal   = 0
	greater = 1
)

type SizePolicyService struct{}

func NewSizePolicyService() *SizePolicyService {
	return &SizePolicyService{}
}

func (s *SizePolicyService) CheckVMMatchedSizePolicy(vm *v1alpha2.VirtualMachine, vmClass *v1alpha2.VirtualMachineClass) error {
	sizePolicy := getVMSizePolicy(vm, vmClass)
	if sizePolicy == nil {
		return fmt.Errorf(
			"virtual machine %q resources do not match any sizing policies in class %q",
			vm.Name, vm.Spec.VirtualMachineClassName,
		)
	}

	var errorsArray []error

	errorsArray = append(errorsArray, validateCoreFraction(vm, sizePolicy)...)
	errorsArray = append(errorsArray, validateMemory(vm, sizePolicy)...)
	errorsArray = append(errorsArray, validatePerCoreMemory(vm, sizePolicy)...)

	if len(errorsArray) > 0 {
		return fmt.Errorf("sizing policy validate: %w", errors.Join(errorsArray...))
	}

	return nil
}

func getVMSizePolicy(vm *v1alpha2.VirtualMachine, vmClass *v1alpha2.VirtualMachineClass) *v1alpha2.SizingPolicy {
	for _, sp := range vmClass.Spec.SizingPolicies {
		if vm.Spec.CPU.Cores >= sp.Cores.Min && vm.Spec.CPU.Cores <= sp.Cores.Max {
			return sp.DeepCopy()
		}
	}

	return nil
}

func validateCoreFraction(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) (errorsArray []error) {
	if sp.CoreFractions == nil {
		return
	}

	fractionStr := strings.ReplaceAll(vm.Spec.CPU.CoreFraction, "%", "")
	fraction, err := strconv.Atoi(fractionStr)
	if err != nil {
		errorsArray = append(errorsArray, fmt.Errorf("unable to parse CPU core fraction: %w", err))
		return
	}

	hasFractionValueInPolicy := false
	for _, spFraction := range sp.CoreFractions {
		if fraction == int(spFraction) {
			hasFractionValueInPolicy = true
		}
	}

	if !hasFractionValueInPolicy {
		errorsArray = append(errorsArray, fmt.Errorf("VM core fraction value %d is not within the allowed values", fraction))
	}

	return
}

func validateMemory(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) (errorsArray []error) {
	if sp.Memory == nil || sp.Memory.Max.IsZero() {
		return
	}

	if vm.Spec.Memory.Size.Cmp(sp.Memory.Min) == lesser {
		errorsArray = append(errorsArray, fmt.Errorf(
			"requested VM memory (%s) is less than the minimum allowed, available range [%s, %s]",
			vm.Spec.Memory.Size.String(),
			sp.Memory.Min.String(),
			sp.Memory.Max.String(),
		))
	}

	if vm.Spec.Memory.Size.Cmp(sp.Memory.Max) == greater {
		errorsArray = append(errorsArray, fmt.Errorf(
			"requested VM memory (%s) exceeds the maximum allowed, available range [%s, %s]",
			vm.Spec.Memory.Size.String(),
			sp.Memory.Min.String(),
			sp.Memory.Max.String(),
		))
	}

	if !sp.Memory.Step.IsZero() {
		err := validateIsQuantized(vm.Spec.Memory.Size, sp.Memory.Min, sp.Memory.Max, sp.Memory.Step, "VM memory")
		if err != nil {
			errorsArray = append(errorsArray, err)
		}
	}

	return
}

func validatePerCoreMemory(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) (errorsArray []error) {
	if sp.Memory == nil || sp.Memory.PerCore.Max.IsZero() {
		return
	}

	// not have a default dividing :(
	// dirty, I know
	// wash your hands after read this
	vmPerCore := vm.Spec.Memory.Size.Value() / int64(vm.Spec.CPU.Cores)
	perCoreMemory := resource.NewQuantity(vmPerCore, resource.BinarySI)

	if perCoreMemory.Cmp(sp.Memory.PerCore.Min) == lesser {
		errorsArray = append(errorsArray, fmt.Errorf(
			"requested VM per core memory (%s) is less than the minimum allowed, available range [%s, %s]",
			perCoreMemory.String(),
			sp.Memory.PerCore.Min.String(),
			sp.Memory.PerCore.Max.String(),
		))
	}

	if perCoreMemory.Cmp(sp.Memory.PerCore.Max) == greater {
		errorsArray = append(errorsArray, fmt.Errorf(
			"requested VM per core memory (%s) exceeds the maximum allowed, available range [%s, %s]",
			perCoreMemory.String(),
			sp.Memory.PerCore.Min.String(),
			sp.Memory.PerCore.Max.String(),
		))
	}

	if !sp.Memory.Step.IsZero() {
		err := validateIsQuantized(*perCoreMemory, sp.Memory.PerCore.Min, sp.Memory.PerCore.Max, sp.Memory.Step, "VM per core memory")
		if err != nil {
			errorsArray = append(errorsArray, err)
		}
	}

	return
}

func validateIsQuantized(value, min, max, step resource.Quantity, source string) (err error) {
	grid := generateValidGrid(min, max, step)

	for i := 0; i < len(grid)-1; i++ {
		cmpLeftResult := value.Cmp(grid[i])
		cmpRightResult := value.Cmp(grid[i+1])

		if cmpLeftResult == equal || cmpRightResult == equal {
			return
		} else if cmpLeftResult == greater && cmpRightResult == lesser {
			err = fmt.Errorf(
				"requested %s does not match any available values, nearest valid values are [%s, %s]",
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
	var grid []resource.Quantity

	for val := min; val.Cmp(max) == lesser; val.Add(step) {
		grid = append(grid, val)
	}

	grid = append(grid, max)

	return grid
}