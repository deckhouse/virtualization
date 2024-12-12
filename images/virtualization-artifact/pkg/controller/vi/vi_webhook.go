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

package vi

import (
	"context"
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type Validator struct {
	logger *log.Logger
}

func NewValidator(logger *log.Logger) *Validator {
	return &Validator{
		logger: logger.With("webhook", "validator"),
	}
}

func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: create operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVI, ok := oldObj.(*virtv2.VirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualImage but got a %T", newObj)
	}

	newVI, ok := newObj.(*virtv2.VirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualImage but got a %T", newObj)
	}

	v.logger.Info("Validating VirtualImage")

	if oldVI.Generation == newVI.Generation {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(vicondition.ReadyType, newVI.Status.Conditions)
	if ready.Status == metav1.ConditionTrue || newVI.Status.Phase == virtv2.ImageReady || newVI.Status.Phase == virtv2.ImageLost || newVI.Status.Phase == virtv2.ImageTerminating {
		if !reflect.DeepEqual(oldVI.Spec.DataSource, newVI.Spec.DataSource) {
			return nil, fmt.Errorf("VirtualImage has already been created: data source cannot be changed after disk is created")
		}

		if !reflect.DeepEqual(oldVI.Spec.PersistentVolumeClaim.StorageClass, newVI.Spec.PersistentVolumeClaim.StorageClass) {
			return nil, fmt.Errorf("VirtualImage has already been created: storage class cannot be changed after disk is created")
		}
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
