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

package vdsnapshot

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Validator struct {
	logger *log.Logger
}

func NewValidator(logger *log.Logger) *Validator {
	return &Validator{
		logger: logger.With("webhook", "validator"),
	}
}

func (v *Validator) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVDS, ok := oldObj.(*v1alpha2.VirtualDiskSnapshot)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualDiskSnapshot but got a %T", newObj)
	}

	newVDS, ok := newObj.(*v1alpha2.VirtualDiskSnapshot)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualDiskSnapshot but got a %T", newObj)
	}

	v.logger.Info("Validating VirtualDiskSnapshot")

	if oldVDS.Generation != newVDS.Generation {
		return nil, fmt.Errorf("VirtualDiskSnapshot is an idempotent resource: specification changes are not available")
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
