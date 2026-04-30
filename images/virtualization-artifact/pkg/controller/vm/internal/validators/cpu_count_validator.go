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

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CPUCountValidator struct{}

func NewCPUCountValidator() *CPUCountValidator {
	return &CPUCountValidator{}
}

func (v *CPUCountValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(vm)
}

func (v *CPUCountValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(newVM)
}

func (v *CPUCountValidator) Validate(vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	cores := vm.Spec.CPU.Cores

	if cores > commonvm.MaxCores {
		return nil, fmt.Errorf("number of cores should not exceed %d", commonvm.MaxCores)
	}

	sockets, coresPerSocket, _ := commonvm.CalculateCoresAndSockets(cores)

	if cores == sockets*coresPerSocket {
		return nil, nil
	}

	return nil, fmt.Errorf("the requested number of cores must be a multiple of %d", sockets)
}
