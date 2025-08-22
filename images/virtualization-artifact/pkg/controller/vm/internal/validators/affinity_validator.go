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
	"fmt"

	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	k8sUtils "github.com/deckhouse/virtualization-controller/pkg/controller/k8s-validation"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type AffinityValidator struct{}

func NewAffinityValidator() *AffinityValidator {
	return &AffinityValidator{}
}

func (v *AffinityValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(vm)
}

func (v *AffinityValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(newVM)
}

func (v *AffinityValidator) Validate(vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	errs := k8sUtils.ValidateAffinity(vm.Spec.Affinity, k8sfield.NewPath("spec"))

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors while validating affinity: %w", errs.ToAggregate())
	}

	return nil, nil
}
