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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SizingPolicyValidator struct {
	client  client.Client
	service *service.SizePolicyService
}

func NewSizingPolicyValidator(client client.Client) *SizingPolicyValidator {
	return &SizingPolicyValidator{
		client:  client,
		service: service.NewSizePolicyService(),
	}
}

func (v *SizingPolicyValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	vmClass := &v1alpha2.VirtualMachineClass{}
	err := v.client.Get(ctx, types.NamespacedName{
		Name: vm.Spec.VirtualMachineClassName,
	}, vmClass)
	if err != nil {
		return nil, err
	}

	return nil, v.service.CheckVMMatchedSizePolicy(vm, vmClass)
}

func (v *SizingPolicyValidator) ValidateUpdate(ctx context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	vmClass := &v1alpha2.VirtualMachineClass{}
	err := v.client.Get(ctx, types.NamespacedName{
		Name: newVM.Spec.VirtualMachineClassName,
	}, vmClass)
	if err != nil {
		return nil, err
	}

	return nil, v.service.CheckVMMatchedSizePolicy(newVM, vmClass)
}
