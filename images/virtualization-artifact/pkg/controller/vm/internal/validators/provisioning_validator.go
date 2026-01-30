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

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ProvisioningValidator struct {
	provisioningService *service.ProvisioningService
}

func NewProvisioningValidator(provisioningService *service.ProvisioningService) *ProvisioningValidator {
	return &ProvisioningValidator{provisioningService: provisioningService}
}

func (p *ProvisioningValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	err := p.validateUserDataLen(vm)
	if err != nil {
		return admission.Warnings{}, err
	}

	return nil, nil
}

func (p *ProvisioningValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	err := p.validateUserDataLen(newVM)
	if err != nil {
		return admission.Warnings{}, err
	}

	return nil, nil
}

func (p *ProvisioningValidator) validateUserDataLen(vm *v1alpha2.VirtualMachine) error {
	if vm.Spec.Provisioning != nil && vm.Spec.Provisioning.Type == v1alpha2.ProvisioningTypeUserData {
		err := p.provisioningService.ValidateUserDataLen(vm.Spec.Provisioning.UserData)
		if err != nil {
			return err
		}
	}
	return nil
}
