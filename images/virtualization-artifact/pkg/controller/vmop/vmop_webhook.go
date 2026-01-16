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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/validator"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewValidator(c client.Client, log *log.Logger) admission.CustomValidator {
	return validator.NewValidator[*v1alpha2.VirtualMachineOperation](log.
		With("controller", "vmop-controller").
		With("webhook", "validation"),
	).WithCreateValidators(&deprecateMigrateValidator{})
}

type deprecateMigrateValidator struct{}

func (v *deprecateMigrateValidator) ValidateCreate(_ context.Context, vmop *v1alpha2.VirtualMachineOperation) (admission.Warnings, error) {
	// TODO: Delete me after v0.15
	if vmop.Spec.Type == v1alpha2.VMOPTypeMigrate {
		return admission.Warnings{"The Migrate type is deprecated, consider using Evict operation"}, nil
	}

	return admission.Warnings{}, nil
}
