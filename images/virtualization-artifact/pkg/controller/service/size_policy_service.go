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

	"github.com/deckhouse/virtualization-controller/pkg/common"
	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var ErrSizingPolicyValidation = errors.New("check the sizing policy of the VirtualMachineClass or contact the administrator for more information")

type SizePolicyService struct{}

func NewSizePolicyService() *SizePolicyService {
	return &SizePolicyService{}
}

func (s *SizePolicyService) CheckVMMatchedSizePolicy(vm *v1alpha2.VirtualMachine, vmClass *v1alpha2.VirtualMachineClass) error {
	if vmClass == nil || len(vmClass.Spec.SizingPolicies) == 0 {
		return nil
	}

	sizePolicy := getVMSizePolicy(vm, vmClass)
	if sizePolicy == nil {
		return NewNoSizingPolicyMatchError(vm.Spec.CPU.Cores, vmClass.GetName(), collectCoreRanges(vmClass))
	}

	var errs []error

	if err := validateCoreFraction(vm, sizePolicy); err != nil {
		errs = append(errs, err)
	}

	if err := validateCores(vm, sizePolicy); err != nil {
		errs = append(errs, err)
	}

	if err := validateMemory(vm, sizePolicy); err != nil {
		errs = append(errs, err)
	}

	if err := validatePerCoreMemory(vm, sizePolicy); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil
	}

	return newSizePolicyValidationError(vmClass.GetName(), errs)
}

// newSizePolicyValidationError renders one or more validation failures as a single
// user-facing message and appends the shared hint on where to look next.
func newSizePolicyValidationError(className string, errs []error) error {
	if len(errs) == 1 {
		return fmt.Errorf("does not match the sizing policy of VirtualMachineClass %q: %w: %w", className, errs[0], ErrSizingPolicyValidation)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "does not match the sizing policy of VirtualMachineClass %q for several reasons:", className)
	for _, err := range errs {
		fmt.Fprintf(&b, "\n  - %s", err.Error())
	}

	return fmt.Errorf("%s\n%w", b.String(), ErrSizingPolicyValidation)
}

func getVMSizePolicy(vm *v1alpha2.VirtualMachine, vmClass *v1alpha2.VirtualMachineClass) *v1alpha2.SizingPolicy {
	for _, sp := range vmClass.Spec.SizingPolicies {
		if sp.Cores == nil {
			continue
		}

		if vm.Spec.CPU.Cores >= sp.Cores.Min && vm.Spec.CPU.Cores <= sp.Cores.Max {
			return sp.DeepCopy()
		}
	}

	return nil
}

func collectCoreRanges(vmClass *v1alpha2.VirtualMachineClass) []CoreRange {
	var ranges []CoreRange
	for _, sp := range vmClass.Spec.SizingPolicies {
		if sp.Cores == nil {
			continue
		}
		ranges = append(ranges, CoreRange{Min: sp.Cores.Min, Max: sp.Cores.Max})
	}
	return ranges
}

func validateCoreFraction(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) error {
	if len(sp.CoreFractions) == 0 {
		return nil
	}

	fractionStr, _ := strings.CutSuffix(vm.Spec.CPU.CoreFraction, "%")
	fraction, err := strconv.Atoi(fractionStr)
	if err != nil {
		return fmt.Errorf("unable to parse the CPU core fraction %q", vm.Spec.CPU.CoreFraction)
	}

	for _, spFraction := range sp.CoreFractions {
		if fraction == int(spFraction) {
			return nil
		}
	}

	formattedCoreFractions := sizingpolicy.FormatCoreFractionValues(sp.CoreFractions)
	return fmt.Errorf(
		"the CPU core fraction %q is not allowed; set the core fraction (spec.cpu.coreFraction) to one of: %s",
		vm.Spec.CPU.CoreFraction,
		strings.Join(formattedCoreFractions, ", "),
	)
}

// validateCores checks that the requested number of CPU cores matches the
// discretization step of the sizing policy (the policy is already selected by
// the cores range, so only the step has to be verified here).
func validateCores(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) error {
	if sp.Cores == nil || sp.Cores.Step <= 0 {
		return nil
	}

	cores := vm.Spec.CPU.Cores
	grid := generateValidCoreGrid(sp.Cores.Min, sp.Cores.Max, sp.Cores.Step)

	for _, v := range grid {
		if v == cores {
			return nil
		}
	}

	lower, upper := grid[0], grid[len(grid)-1]
	for i := 0; i < len(grid)-1; i++ {
		if cores > grid[i] && cores < grid[i+1] {
			lower, upper = grid[i], grid[i+1]
			break
		}
	}

	return fmt.Errorf(
		"the number of CPU cores (%d) does not match the sizing policy step; set the number of cores (spec.cpu.cores) to %d or %d",
		cores, lower, upper,
	)
}

func validateMemory(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) error {
	if sp.Memory == nil {
		return nil
	}

	size := vm.Spec.Memory.Size
	min := sp.Memory.Min
	max := sp.Memory.Max
	minSet := min != nil && !min.IsZero()
	maxSet := max != nil && !max.IsZero()

	if minSet && size.Cmp(*min) == common.CmpLesser {
		return memoryOutOfRangeError(size, min, max)
	}

	if maxSet && size.Cmp(*max) == common.CmpGreater {
		return memoryOutOfRangeError(size, min, max)
	}

	// The step grid needs a finite upper bound, so it is only checked when max is set.
	if maxSet && sp.Memory.Step != nil && !sp.Memory.Step.IsZero() {
		minVal := resource.Quantity{}
		if minSet {
			minVal = *min
		}
		if lower, upper, ok := validateIsQuantized(size, minVal, *max, *sp.Memory.Step); !ok {
			return fmt.Errorf(
				"the memory size (%s) does not match the sizing policy step; set the memory size (spec.memory.size) to %s or %s",
				size.String(), lower.String(), upper.String(),
			)
		}
	}

	return nil
}

// validatePerCoreMemory validates the per-core memory limits. The policy expresses
// them per CPU core, but the user sets the total memory (spec.memory.size), so all
// messages report the total values for the current number of cores.
func validatePerCoreMemory(vm *v1alpha2.VirtualMachine, sp *v1alpha2.SizingPolicy) error {
	if sp.Memory == nil || sp.Memory.PerCore == nil {
		return nil
	}

	cores := int64(vm.Spec.CPU.Cores)
	if cores <= 0 {
		return nil
	}

	perCoreMin := sp.Memory.PerCore.Min
	perCoreMax := sp.Memory.PerCore.Max
	minSet := perCoreMin != nil && !perCoreMin.IsZero()
	maxSet := perCoreMax != nil && !perCoreMax.IsZero()
	if !minSet && !maxSet {
		return nil
	}

	// Calculate the memory portion per CPU core to compare it with the policy limits.
	perCoreMemory := resource.NewQuantity(vm.Spec.Memory.Size.Value()/cores, resource.BinarySI)
	size := vm.Spec.Memory.Size

	if minSet && perCoreMemory.Cmp(*perCoreMin) == common.CmpLesser {
		return perCoreMemoryOutOfRangeError(size, perCoreMin, perCoreMax, vm.Spec.CPU.Cores)
	}

	if maxSet && perCoreMemory.Cmp(*perCoreMax) == common.CmpGreater {
		return perCoreMemoryOutOfRangeError(size, perCoreMin, perCoreMax, vm.Spec.CPU.Cores)
	}

	if maxSet && sp.Memory.Step != nil && !sp.Memory.Step.IsZero() {
		minVal := resource.Quantity{}
		if minSet {
			minVal = *perCoreMin
		}
		if lower, upper, ok := validateIsQuantized(*perCoreMemory, minVal, *perCoreMax, *sp.Memory.Step); !ok {
			lowerTotal := scaleQuantity(lower, cores)
			upperTotal := scaleQuantity(upper, cores)
			return fmt.Errorf(
				"the memory size (%s) does not match the per-core sizing policy step for %d CPU core(s); set the memory size (spec.memory.size) to %s or %s, or change the number of cores (spec.cpu.cores)",
				size.String(), vm.Spec.CPU.Cores, lowerTotal.String(), upperTotal.String(),
			)
		}
	}

	return nil
}

func memoryOutOfRangeError(size resource.Quantity, min, max *resource.Quantity) error {
	return fmt.Errorf(
		"the memory size (%s) is out of the range allowed by the sizing policy; %s",
		size.String(), setMemoryClause(min, max),
	)
}

func perCoreMemoryOutOfRangeError(size resource.Quantity, perCoreMin, perCoreMax *resource.Quantity, cores int) error {
	var minTotal, maxTotal *resource.Quantity
	if perCoreMin != nil {
		q := scaleQuantity(*perCoreMin, int64(cores))
		minTotal = &q
	}
	if perCoreMax != nil {
		q := scaleQuantity(*perCoreMax, int64(cores))
		maxTotal = &q
	}

	return fmt.Errorf(
		"the memory size (%s) is not allowed for %d CPU core(s); %s, or change the number of cores (spec.cpu.cores) (the sizing policy allows %s of memory per core)",
		size.String(), cores, setMemoryClause(minTotal, maxTotal), memoryRangeClause(perCoreMin, perCoreMax),
	)
}

// setMemoryClause builds an actionable directive telling the user which total
// memory size to set, based on which bounds the policy defines.
func setMemoryClause(min, max *resource.Quantity) string {
	switch {
	case min != nil && max != nil:
		return fmt.Sprintf("set the memory size (spec.memory.size) between %s and %s", min.String(), max.String())
	case min != nil:
		return fmt.Sprintf("set the memory size (spec.memory.size) to at least %s", min.String())
	case max != nil:
		return fmt.Sprintf("set the memory size (spec.memory.size) to at most %s", max.String())
	default:
		return ""
	}
}

// memoryRangeClause describes an allowed range without an imperative, used as a
// supplementary note (for example, the per-core allowance).
func memoryRangeClause(min, max *resource.Quantity) string {
	switch {
	case min != nil && max != nil:
		return fmt.Sprintf("between %s and %s", min.String(), max.String())
	case min != nil:
		return fmt.Sprintf("at least %s", min.String())
	case max != nil:
		return fmt.Sprintf("at most %s", max.String())
	default:
		return ""
	}
}

func scaleQuantity(q resource.Quantity, factor int64) resource.Quantity {
	return *resource.NewQuantity(q.Value()*factor, resource.BinarySI)
}

// validateIsQuantized reports whether value sits on the min..max grid with the given step.
// When it does not, it returns the nearest lower and upper grid values.
func validateIsQuantized(value, min, max, step resource.Quantity) (lower, upper resource.Quantity, ok bool) {
	grid := generateValidGrid(min, max, step)

	for i := 0; i < len(grid)-1; i++ {
		cmpLeftResult := value.Cmp(grid[i])
		cmpRightResult := value.Cmp(grid[i+1])

		if cmpLeftResult == common.CmpEqual || cmpRightResult == common.CmpEqual {
			return resource.Quantity{}, resource.Quantity{}, true
		}

		if cmpLeftResult == common.CmpGreater && cmpRightResult == common.CmpLesser {
			return grid[i], grid[i+1], false
		}
	}

	return resource.Quantity{}, resource.Quantity{}, true
}

func generateValidGrid(min, max, step resource.Quantity) []resource.Quantity {
	var grid []resource.Quantity

	for val := min; val.Cmp(max) == common.CmpLesser; val.Add(step) {
		grid = append(grid, val)
	}

	grid = append(grid, max)

	return grid
}

func generateValidCoreGrid(min, max, step int) []int {
	var grid []int

	for v := min; v < max; v += step {
		grid = append(grid, v)
	}

	grid = append(grid, max)

	return grid
}
