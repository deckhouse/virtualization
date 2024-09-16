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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SizingPolicyValidator struct {
	service *service.SizePolicyService
}

func NewSizingPolicyValidator(client *client.Reader) *SizingPolicyValidator {
	return &SizingPolicyValidator{
		service: service.NewSizePolicyService(*client),
	}
}

func (v *SizingPolicyValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.service.CheckVMMatchedSizePolicy(ctx, vm)
}

func (v *SizingPolicyValidator) ValidateUpdate(ctx context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return nil, v.service.CheckVMMatchedSizePolicy(ctx, newVM)
}
