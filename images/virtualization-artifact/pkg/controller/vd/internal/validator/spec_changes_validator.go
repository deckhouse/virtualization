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
	"errors"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type SpecChangesValidator struct {
	scService *intsvc.VirtualDiskStorageClassService
}

func NewSpecChangesValidator(scService *intsvc.VirtualDiskStorageClassService) *SpecChangesValidator {
	return &SpecChangesValidator{
		scService: scService,
	}
}

func (v *SpecChangesValidator) ValidateCreate(ctx context.Context, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	return nil, nil
}

func (v *SpecChangesValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if oldVD.Generation == newVD.Generation {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, newVD.Status.Conditions)
	switch {
	case ready.Status == metav1.ConditionTrue, newVD.Status.Phase == v1alpha2.DiskReady, newVD.Status.Phase == v1alpha2.DiskMigrating, newVD.Status.Phase == v1alpha2.DiskLost:
		if !reflect.DeepEqual(oldVD.Spec.DataSource, newVD.Spec.DataSource) {
			return nil, errors.New("data source cannot be changed if the VirtualDisk has already been provisioned")
		}

	case newVD.Status.Phase == v1alpha2.DiskTerminating:
		if !reflect.DeepEqual(oldVD.Spec, newVD.Spec) {
			return nil, errors.New("spec cannot be changed if the VirtualDisk is the process of termination")
		}
	}

	return nil, nil
}
