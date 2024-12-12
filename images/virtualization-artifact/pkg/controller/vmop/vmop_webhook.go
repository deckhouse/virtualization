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

package vmop

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewValidator(log *log.Logger) *Validator {
	return &Validator{
		log: log.With("controller", ControllerName).With("webhook", "validation"),
	}
}

type Validator struct {
	log *log.Logger
}

func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	vmop, ok := obj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineOperation but got a %T", obj)
	}

	// TODO: Delete me after v0.15
	if vmop.Spec.Type == v1alpha2.VMOPTypeMigrate {
		return admission.Warnings{"The Migrate type is deprecated, consider using Evict operation"}, nil
	}

	return admission.Warnings{}, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVmop, ok := oldObj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineOperation but got a %T", oldObj)
	}
	newVmop, ok := newObj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineOperation but got a %T", newObj)
	}

	v.log.Info("Validate VMOP updating", "name", oldVmop.GetName())

	if oldVmop.Generation == newVmop.Generation {
		return nil, nil
	}

	// spec changes are not allowed.
	err := fmt.Errorf("recreate VirtualMachineOperation/%s to apply changes: .spec modification is not allowed after creation", oldVmop.GetName())
	return nil, err
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
