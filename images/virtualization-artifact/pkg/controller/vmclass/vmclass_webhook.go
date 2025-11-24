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

package vmclass

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/validators"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

type VirtualMachineClassValidator interface {
	ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachineClass) (admission.Warnings, error)
	ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachineClass) (admission.Warnings, error)
}

type Validator struct {
	validators []VirtualMachineClassValidator
	log        *log.Logger
}

func NewValidator(client client.Client, log *log.Logger, recorder eventrecord.EventRecorderLogger, vmClassService *service.VirtualMachineClassService) *Validator {
	return &Validator{
		validators: []VirtualMachineClassValidator{
			validators.NewSizingPoliciesValidator(client),
			validators.NewPolicyChangesValidator(recorder),
			validators.NewSingleDefaultClassValidator(client, vmClassService),
		},
		log: log.With("webhook", "validation"),
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var vmclass *v1alpha2.VirtualMachineClass

	switch o := obj.(type) {
	case *v1alpha2.VirtualMachineClass:
		vmclass = o
	case *v1alpha3.VirtualMachineClass:
		vmclass = &v1alpha2.VirtualMachineClass{}
		if err := o.ConvertTo(vmclass); err != nil {
			return nil, fmt.Errorf("failed to convert v1alpha3 to v1alpha2: %w", err)
		}
	default:
		return nil, fmt.Errorf("expected a VirtualMachineClass but got a %T", obj)
	}

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateCreate(ctx, vmclass)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var oldVMClass, newVMClass *v1alpha2.VirtualMachineClass

	switch o := oldObj.(type) {
	case *v1alpha2.VirtualMachineClass:
		oldVMClass = o
	case *v1alpha3.VirtualMachineClass:
		oldVMClass = &v1alpha2.VirtualMachineClass{}
		if err := o.ConvertTo(oldVMClass); err != nil {
			return nil, fmt.Errorf("failed to convert old v1alpha3 to v1alpha2: %w", err)
		}
	default:
		return nil, fmt.Errorf("expected an old VirtualMachineClass but got a %T", oldObj)
	}

	switch n := newObj.(type) {
	case *v1alpha2.VirtualMachineClass:
		newVMClass = n
	case *v1alpha3.VirtualMachineClass:
		newVMClass = &v1alpha2.VirtualMachineClass{}
		if err := n.ConvertTo(newVMClass); err != nil {
			return nil, fmt.Errorf("failed to convert new v1alpha3 to v1alpha2: %w", err)
		}
	default:
		return nil, fmt.Errorf("expected a new VirtualMachineClass but got a %T", newObj)
	}

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateUpdate(ctx, oldVMClass, newVMClass)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err.Error())
	return nil, nil
}
