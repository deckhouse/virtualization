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
	"strconv"
	"strings"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/util"
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

	sizePolicy := &v1alpha2.SizingPolicy{}
	isCoreMatched := false
	for _, sp := range vmclass.Spec.SizingPolicies {
		if vm.Spec.CPU.Cores >= sp.Cores.Min && vm.Spec.CPU.Cores <= sp.Cores.Max {
			isCoreMatched = true
			sizePolicy = sp.DeepCopy()
			break
		}
	}
	if !isCoreMatched {
		return fmt.Errorf(
			"virtual machine %s has no valid size policy in vm class %s",
			vm.Name, vm.Spec.VirtualMachineClassName,
		)
	}

	errors := make([]string, 0)

	canValidateMemory := true
	canValidateGlobalMemory := true
	canValidatePerCoreMemory := true

	if sizePolicy.Memory != nil {
		vmMemory, ok := vm.Spec.Memory.Size.AsInt64()
		if !ok {
			canValidateMemory = false
			errors = append(errors, "invalid virtual machine memory value")
		}
		spMemoryMin, ok := sizePolicy.Memory.Min.AsInt64()
		if !ok {
			canValidateGlobalMemory = false
			errors = append(errors, "invalid size policy min memory value")
		}
		spMemoryMax, ok := sizePolicy.Memory.Max.AsInt64()
		if !ok {
			canValidateGlobalMemory = false
			errors = append(errors, "invalid size policy max memory value")
		}

		// if exists perCoreMemory then exists Memory field and it is only way I find
		// to check no requirements for vm memory
		if spMemoryMax == 0 {
			canValidateGlobalMemory = false
		}

		if canValidateMemory && canValidateGlobalMemory {
			if vmMemory < spMemoryMin {
				errors = append(errors, fmt.Sprintf(
					"virtual machine setted memory(%s) lesser than size policy min memory value(%s)",
					util.HumanizeIBytes(uint64(vmMemory)),
					util.HumanizeIBytes(uint64(spMemoryMin)),
				))
			}

			if vmMemory > spMemoryMax {
				errors = append(errors, fmt.Sprintf(
					"virtual machine setted memory(%s) greater than size policy max memory value(%s)",
					util.HumanizeIBytes(uint64(vmMemory)),
					util.HumanizeIBytes(uint64(spMemoryMax)),
				))
			}
		}

		spPerCoreMemoryMin, ok := sizePolicy.Memory.PerCore.Min.AsInt64()
		if !ok {
			canValidatePerCoreMemory = false
			errors = append(errors, "invalid size policy min per core memory value")
		}
		spPerCoreMemoryMax, ok := sizePolicy.Memory.PerCore.Max.AsInt64()
		if !ok {
			canValidatePerCoreMemory = false
			errors = append(errors, "invalid size policy max per core memory value")
		}

		// value can't be nil, check rude
		if spPerCoreMemoryMax == 0 {
			canValidatePerCoreMemory = false
		}

		if canValidateMemory && canValidatePerCoreMemory {
			vmPerCoreMemory := vmMemory / int64(vm.Spec.CPU.Cores)

			if vmPerCoreMemory < spPerCoreMemoryMin {
				errors = append(errors, fmt.Sprintf(
					"virtual machine setted per core memory(%s) lesser than size policy min per core memory value(%s)",
					util.HumanizeIBytes(uint64(vmPerCoreMemory)),
					util.HumanizeIBytes(uint64(spPerCoreMemoryMin)),
				))
			}

			if vmPerCoreMemory > spPerCoreMemoryMax {
				errors = append(errors, fmt.Sprintf(
					"virtual machine setted per core memory(%s) greater than size policy max per core memory value(%s)",
					util.HumanizeIBytes(uint64(vmPerCoreMemory)),
					util.HumanizeIBytes(uint64(spPerCoreMemoryMax)),
				))
			}
		}
	}

	if sizePolicy.CoreFractions != nil {
		fractionStr := strings.ReplaceAll(vm.Spec.CPU.CoreFraction, "%", "")
		fraction, err := strconv.Atoi(fractionStr)
		if err != nil {
			errors = append(errors, "cpu fraction value is invalid")
		}

		hasInSizePolicyFractions := false
		for _, spFraction := range sizePolicy.CoreFractions {
			if fraction == int(spFraction) {
				hasInSizePolicyFractions = true
			}
		}

		if !hasInSizePolicyFractions {
			errors = append(errors, "vm core fraction incorrect")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, ", "))
	}

	return nil
}
