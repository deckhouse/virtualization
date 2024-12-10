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
	"errors"
	"fmt"

	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	k8sUtils "github.com/deckhouse/virtualization-controller/pkg/controller/k8s-validation"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type TopologySpreadConstraintValidator struct{}

func NewTopologySpreadConstraintValidator() *TopologySpreadConstraintValidator {
	return &TopologySpreadConstraintValidator{}
}

func (v *TopologySpreadConstraintValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(vm)
}

func (v *TopologySpreadConstraintValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(newVM)
}

func (v *TopologySpreadConstraintValidator) Validate(vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	var errs []error

	errorList := k8sUtils.ValidateTopologySpreadConstraints(
		vm.Spec.TopologySpreadConstraints,
		k8sfield.NewPath("spec").Child("topologySpreadConstraints"),
	)
	for _, err := range errorList {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors while validating affinity: %w", errors.Join(errs...))
	}

	return nil, nil
}
