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

package vmsnapshot

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) ValidateCreate(ctx context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: create operation not implemented")
	logger.FromContext(ctx).Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVMSnapshot, ok := oldObj.(*v1alpha2.VirtualMachineSnapshot)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineSnapshot but got a %T", newObj)
	}

	newVMSnapshot, ok := newObj.(*v1alpha2.VirtualMachineSnapshot)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineSnapshot but got a %T", newObj)
	}

	logger.FromContext(ctx).Info("Validating VirtualMachineSnapshot")

	if oldVMSnapshot.Generation != newVMSnapshot.Generation {
		return nil, fmt.Errorf("VirtualMachineSnapshot is an idempotent resource: specification changes are not available")
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(ctx context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	logger.FromContext(ctx).Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
