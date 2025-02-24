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

	vmcommon "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CpuCountValidator struct{}

func NewCpuCountValidator() *CpuCountValidator {
	return &CpuCountValidator{}
}

func (v *CpuCountValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(vm)
}

func (v *CpuCountValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(newVM)
}

func (v *CpuCountValidator) Validate(vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	cores := vm.Spec.CPU.Cores

	switch {
	case cores <= vmcommon.MaxCoresPerSocket:
		return nil, nil
	case cores <= vmcommon.MaxCoresFor2Sockets && cores%vmcommon.SocketsForUpTo32Cores != 0:
		return nil, fmt.Errorf("the requested number of cores must be a multiple of 2")
	case cores <= vmcommon.MaxCoresFor4Sockets && cores%vmcommon.SocketsForUpTo64Cores != 0:
		return nil, fmt.Errorf("the requested number of cores must be a multiple of 4")
	case cores%vmcommon.SocketsForMoreThan64Cores != 0:
		return nil, fmt.Errorf("the requested number of cores must be a multiple of 8")
	}

	return nil, nil
}
