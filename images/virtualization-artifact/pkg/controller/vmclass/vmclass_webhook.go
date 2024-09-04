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
	"log/slog"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineClassValidator interface {
	ValidateCreate(ctx context.Context, vm *virtv2.VirtualMachineClass) (admission.Warnings, error)
	ValidateUpdate(ctx context.Context, oldVM, newVM *virtv2.VirtualMachineClass) (admission.Warnings, error)
}

type Validator struct {
	validators []VirtualMachineClassValidator
	log        *slog.Logger
}

func NewValidator(client client.Client, log *slog.Logger) *Validator {
	return &Validator{
		validators: []VirtualMachineClassValidator{},
		log:        log.With("webhook", "validation"),
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	v.log.Info("kek")
	return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", obj)
	// _, ok := obj.(*virtv2.VirtualMachineClass)
	// if !ok {
	// 	return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", obj)
	// }

	// var warnings admission.Warnings

	// // for _, validator := range v.validators {
	// // 	warn, err := validator.ValidateCreate(ctx, vmclass)
	// // 	if err != nil {
	// // 		return nil, err
	// // 	}
	// // 	warnings = append(warnings, warn...)
	// // }

	// return warnings, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	_, ok := oldObj.(*virtv2.VirtualMachineClass)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineClass but got a %T", oldObj)
	}

	_, ok = newObj.(*virtv2.VirtualMachineClass)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineClass but got a %T", newObj)
	}

	// v.log.Info("Validating VM",
	// 	"old.spec.virtualMachineIPAddress", oldVM.Spec.VirtualMachineIPAddress,
	// 	"new.spec.virtualMachineIPAddress", newVM.Spec.VirtualMachineIPAddress,
	// )

	var warnings admission.Warnings

	// for _, validator := range v.validators {
	// 	warn, err := validator.ValidateUpdate(ctx, oldVM, newVM)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	warnings = append(warnings, warn...)
	// }

	return warnings, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err.Error())
	return nil, nil
}
