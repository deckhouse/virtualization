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

package vmbda

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Validator struct {
	logger *slog.Logger
}

func NewValidator() *Validator {
	return &Validator{
		logger: slog.Default().With("controller", "vmbda", "webhook", "validator"),
	}
}

func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: create operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVMBDA, ok := oldObj.(*virtv2.VirtualMachineBlockDeviceAttachment)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineBlockDeviceAttachment but got a %T", newObj)
	}

	newVMBDA, ok := newObj.(*virtv2.VirtualMachineBlockDeviceAttachment)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineBlockDeviceAttachment but got a %T", newObj)
	}

	v.logger.Info("Validating VirtualMachineBlockDeviceAttachment")

	if oldVMBDA.Generation != newVMBDA.Generation {
		return nil, fmt.Errorf("VirtualMachineBlockDeviceAttachment is an idempotent resource: specification changes are not available")
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
