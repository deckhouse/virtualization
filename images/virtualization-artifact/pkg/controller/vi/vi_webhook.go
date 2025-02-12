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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/blockdevice"
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
	vi, ok := obj.(*virtv2.VirtualImage)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", obj)
	}

	if strings.Contains(vi.Name, ".") {
		return nil, fmt.Errorf("the VirtualImage name %q is invalid: '.' is forbidden, allowed name symbols are [0-9a-zA-Z-]", vi.Name)
	}

	if len(vi.Name) > blockdevice.MaxVirtualImageNameLen {
		return nil, fmt.Errorf("the VirtualImage name %q is too long: it must be no more than %d characters", vi.Name, blockdevice.MaxVirtualImageNameLen)
	}

	if vi.Spec.Storage == virtv2.StorageKubernetes {
		warnings := admission.Warnings{
			fmt.Sprintf("Using the `%s` storage type is deprecated. It is recommended to use `%s` instead.",
				virtv2.StorageKubernetes, virtv2.StoragePersistentVolumeClaim),
		}
		return warnings, nil
	}

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

	var warnings admission.Warnings

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

	if strings.Contains(newVI.Name, ".") {
		warnings = append(warnings, fmt.Sprintf(" the VirtualImage name %q is invalid as it contains now forbidden symbol '.', allowed symbols for name are [0-9a-zA-Z-]. Create another image with valid name to avoid problems with future updates.", newVI.Name))
	}

	if len(newVI.Name) > blockdevice.MaxVirtualImageNameLen {
		warnings = append(warnings, fmt.Sprintf("the VirtualImage name %q is too long: it must be no more than %d characters", newVI.Name, blockdevice.MaxVirtualImageNameLen))
	}

	return warnings, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.logger.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
