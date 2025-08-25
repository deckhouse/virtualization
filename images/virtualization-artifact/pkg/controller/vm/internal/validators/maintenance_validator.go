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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type MaintenanceValidator struct{}

func NewMaintenanceValidator() *MaintenanceValidator {
	return &MaintenanceValidator{}
}

func (v *MaintenanceValidator) ValidateCreate(_ context.Context, _ *virtv2.VirtualMachine) (admission.Warnings, error) {
	return nil, nil
}

func (v *MaintenanceValidator) ValidateUpdate(_ context.Context, oldVM, newVM *virtv2.VirtualMachine) (admission.Warnings, error) {
	maintenance, _ := conditions.GetCondition(vmcondition.TypeMaintenance, oldVM.Status.Conditions)

	if maintenance.Status != metav1.ConditionTrue {
		return nil, nil
	}

	if !reflect.DeepEqual(oldVM.Spec, newVM.Spec) {
		return nil, fmt.Errorf("spec changes are not allowed while VirtualMachine is in maintenance mode")
	}

	return nil, nil
}

