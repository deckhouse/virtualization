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

package validator

import (
	"context"
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type SpecChangesValidator struct{}

func NewSpecChangesValidator() *SpecChangesValidator {
	return &SpecChangesValidator{}
}

func (v *SpecChangesValidator) ValidateCreate(_ context.Context, _ *virtv2.VirtualDisk) (admission.Warnings, error) {
	return nil, nil
}

func (v *SpecChangesValidator) ValidateUpdate(_ context.Context, oldVD, newVD *virtv2.VirtualDisk) (admission.Warnings, error) {
	if oldVD.Generation == newVD.Generation {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, newVD.Status.Conditions)
	if ready.Status == metav1.ConditionTrue || newVD.Status.Phase == virtv2.DiskReady || newVD.Status.Phase == virtv2.DiskLost || newVD.Status.Phase == virtv2.DiskTerminating {
		if !reflect.DeepEqual(oldVD.Spec.DataSource, newVD.Spec.DataSource) {
			return nil, fmt.Errorf("VirtualDisk has already been attached: data source cannot be changed while disk is attached to VirtualMachine")
		}

		if !reflect.DeepEqual(oldVD.Spec.PersistentVolumeClaim.StorageClass, newVD.Spec.PersistentVolumeClaim.StorageClass) {
			return nil, fmt.Errorf("VirtualDisk has already been attached: storage class cannot be changed while disk is attached to VirtualMachine")
		}
	}

	return nil, nil
}
