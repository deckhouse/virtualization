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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

type PolicyChangesValidator struct {
	recorder eventrecord.EventRecorderLogger
}

func NewPolicyChangesValidator(recorder eventrecord.EventRecorderLogger) *PolicyChangesValidator {
	return &PolicyChangesValidator{recorder: recorder}
}

func (v *PolicyChangesValidator) ValidateCreate(_ context.Context, _ *v1alpha3.VirtualMachineClass) (admission.Warnings, error) {
	return nil, nil
}

func (v *PolicyChangesValidator) ValidateUpdate(_ context.Context, oldVMClass, newVMClass *v1alpha3.VirtualMachineClass) (admission.Warnings, error) {
	if !reflect.DeepEqual(oldVMClass.Spec.SizingPolicies, newVMClass.Spec.SizingPolicies) {
		v.recorder.Event(newVMClass, corev1.EventTypeNormal, v1alpha2.ReasonVMClassSizingPoliciesWereChanged, "Sizing policies were changed")
	}

	return nil, nil
}
