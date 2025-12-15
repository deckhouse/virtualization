/*
Copyright 2025 Flant JSC

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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SingleDefaultClassValidator struct {
	client         client.Client
	vmClassService *service.VirtualMachineClassService
}

func NewSingleDefaultClassValidator(client client.Client, vmClassService *service.VirtualMachineClassService) *SingleDefaultClassValidator {
	return &SingleDefaultClassValidator{
		client:         client,
		vmClassService: vmClassService,
	}
}

func (v *SingleDefaultClassValidator) ValidateCreate(ctx context.Context, vmClass *v1alpha2.VirtualMachineClass) (admission.Warnings, error) {
	err := v.vmClassService.ValidateDefaultAnnotation(vmClass)
	if err != nil {
		return nil, err
	}

	err = v.checkDefaultIsSingle(ctx, vmClass)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *SingleDefaultClassValidator) ValidateUpdate(ctx context.Context, _, newVMClass *v1alpha2.VirtualMachineClass) (admission.Warnings, error) {
	err := v.vmClassService.ValidateDefaultAnnotation(newVMClass)
	if err != nil {
		return nil, err
	}

	err = v.checkDefaultIsSingle(ctx, newVMClass)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *SingleDefaultClassValidator) checkDefaultIsSingle(ctx context.Context, vmClass *v1alpha2.VirtualMachineClass) error {
	classes := &v1alpha2.VirtualMachineClassList{}
	err := v.client.List(ctx, classes)
	if err != nil {
		return fmt.Errorf("failed to list virtual machine classes: %w", err)
	}

	// Ignore classes without "is-default-class" annotations.
	if !v.vmClassService.IsDefault(vmClass) {
		return nil
	}

	// Prevent adding default class annotation if default class was already assigned.
	for _, vmClassItem := range classes.Items {
		if v.vmClassService.IsDefault(&vmClassItem) && vmClassItem.GetName() != vmClass.GetName() {
			return fmt.Errorf("multiple default virtual machine classes are prohibited, current default class is %s. (tip: to assign new default class first remove '%s' annotation from the current default class)", vmClassItem.GetName(), annotations.AnnVirtualMachineClassDefault)
		}
	}
	return nil
}
