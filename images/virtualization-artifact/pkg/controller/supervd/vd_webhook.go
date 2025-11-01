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

package supervd

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/supervd/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supervd/internal/validator"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualDiskValidator interface {
	ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualDisk) (admission.Warnings, error)
	ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualDisk) (admission.Warnings, error)
}

type Validator struct {
	validators []VirtualDiskValidator
}

func NewValidator(client client.Client, scService *intsvc.VirtualDiskStorageClassService, diskService *service.DiskService) *Validator {
	return &Validator{
		validators: []VirtualDiskValidator{
			validator.NewPVCSizeValidator(client),
			validator.NewSpecChangesValidator(scService),
			// validator.NewISOSourceValidator(client),
			validator.NewNameValidator(),
			validator.NewMigrationStorageClassValidator(client, scService, diskService),
		},
	}
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vd, ok := obj.(*v1alpha2.VirtualDisk)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualDisk but got a %T", obj)
	}

	logger.FromContext(ctx).Info("Validating virtual disk", "spec.pvc.size", vd.Spec.PersistentVolumeClaim.Size)

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateCreate(ctx, vd)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newVD, ok := newObj.(*v1alpha2.VirtualDisk)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualDisk but got a %T", newObj)
	}

	oldVD, ok := oldObj.(*v1alpha2.VirtualDisk)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualDisk but got a %T", oldObj)
	}

	logger.FromContext(ctx).Info("Validating virtual disk",
		"old.spec.pvc.size", oldVD.Spec.PersistentVolumeClaim.Size,
		"new.spec.pvc.size", newVD.Spec.PersistentVolumeClaim.Size,
	)

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.ValidateUpdate(ctx, oldVD, newVD)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *Validator) ValidateDelete(ctx context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	logger.FromContext(ctx).Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
