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

	"github.com/deckhouse/virtualization-controller/pkg/common/cloudinit"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// ProvisioningValidator warns about inline cloud-init user data that looks wrong,
// so the mistake surfaces on apply instead of silently failing inside the guest.
// It only warns and never refuses a machine.
//
// Data kept in a secret is checked by the provisioning handler instead: the secret
// may be created after the machine and may change independently of it.
type ProvisioningValidator struct{}

func NewProvisioningValidator() *ProvisioningValidator {
	return &ProvisioningValidator{}
}

func (v *ProvisioningValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(vm)
}

func (v *ProvisioningValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(newVM)
}

func (v *ProvisioningValidator) Validate(vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	p := vm.Spec.Provisioning
	if p == nil || p.Type != v1alpha2.ProvisioningTypeUserData {
		return nil, nil
	}

	warnings := cloudinit.ValidateUserData([]byte(p.UserData))
	if len(warnings) == 0 {
		return nil, nil
	}

	admissionWarnings := make(admission.Warnings, 0, len(warnings))
	for _, warning := range warnings {
		admissionWarnings = append(admissionWarnings, "spec.provisioning.userData: "+warning)
	}
	return admissionWarnings, nil
}
