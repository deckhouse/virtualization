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

package cpu

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMCPUValidator(log logr.Logger) *VMCPUValidator {
	return &VMCPUValidator{log: log}
}

type VMCPUValidator struct {
	log logr.Logger
}

func (v *VMCPUValidator) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: create operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *VMCPUValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newVMCPU, ok := newObj.(*v1alpha2.VirtualMachineCPUModel)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineCPUModel but got a %T", newObj)
	}

	oldVMCPU, ok := oldObj.(*v1alpha2.VirtualMachineCPUModel)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineCPUModel but got a %T", oldObj)
	}

	if newVMCPU.Spec.Type != oldVMCPU.Spec.Type {
		return nil, errors.New("virtual machine CPU type cannot be changed once set")
	}

	if newVMCPU.Spec.Model != oldVMCPU.Spec.Model {
		return nil, errors.New("virtual machine CPU model cannot be changed once set")
	}

	if !slices.Equal(newVMCPU.Spec.Features, oldVMCPU.Spec.Features) {
		return nil, errors.New("virtual machine CPU features cannot be changed once set")
	}

	return nil, nil
}

func (v *VMCPUValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}
