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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SpecMutateValidator struct{}

func NewSpecMutateValidator() *SpecMutateValidator {
	return &SpecMutateValidator{}
}

func (v *SpecMutateValidator) ValidateCreate(_ context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	return nil, nil
}

func (v *SpecMutateValidator) ValidateUpdate(_ context.Context, oldVMBDA, newVMBDA *virtv2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	if oldVMBDA.Generation != newVMBDA.Generation {
		return nil, fmt.Errorf("VirtualMachineBlockDeviceAttachment is an idempotent resource: specification changes are not available")
	}

	return nil, nil
}
