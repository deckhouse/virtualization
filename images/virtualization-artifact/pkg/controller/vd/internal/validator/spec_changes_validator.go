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
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type SpecChangesValidator struct {
	client    client.Client
	scService *intsvc.VirtualDiskStorageClassService
}

func NewSpecChangesValidator(client client.Client, scService *intsvc.VirtualDiskStorageClassService) *SpecChangesValidator {
	return &SpecChangesValidator{
		client:    client,
		scService: scService,
	}
}

func (v *SpecChangesValidator) ValidateCreate(ctx context.Context, newVD *virtv2.VirtualDisk) (admission.Warnings, error) {
	if newVD.Spec.PersistentVolumeClaim.StorageClass != nil && *newVD.Spec.PersistentVolumeClaim.StorageClass != "" {
		sc, err := v.scService.GetStorageClass(ctx, *newVD.Spec.PersistentVolumeClaim.StorageClass)
		if err != nil {
			return nil, err
		}
		if v.scService.IsStorageClassDeprecated(sc) {
			return nil, fmt.Errorf(
				"the provisioner of the %q storage class is deprecated; please use a different one",
				*newVD.Spec.PersistentVolumeClaim.StorageClass,
			)
		}
		if !v.scService.IsStorageClassAllowed(*newVD.Spec.PersistentVolumeClaim.StorageClass) {
			return nil, fmt.Errorf(
				"the storage class %q is not allowed; please check the module settings",
				*newVD.Spec.PersistentVolumeClaim.StorageClass,
			)
		}
	} else {
		mcDefaultStorageClass, err := v.scService.GetModuleStorageClass(ctx)
		if err != nil && !errors.Is(err, intsvc.ErrStorageClassNotFound) {
			return nil, fmt.Errorf("failed to fetch a default storage class from module config: %w", err)
		}

		if mcDefaultStorageClass == nil {
			defaultStorageClass, err := v.scService.GetDefaultStorageClass(ctx)
			if err != nil && !errors.Is(err, intsvc.ErrStorageClassNotFound) {
				return nil, fmt.Errorf("failed to fetch default storage class: %w", err)
			}

			if defaultStorageClass != nil && !v.scService.IsStorageClassAllowed(defaultStorageClass.Name) {
				return nil, fmt.Errorf(
					"the default storage class %q is not allowed; please check the module settings or specify a storage class name explicitly in the spec",
					defaultStorageClass.Name,
				)
			}
		}
	}

	return nil, nil
}

func (v *SpecChangesValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *virtv2.VirtualDisk) (admission.Warnings, error) {
	if oldVD.Generation == newVD.Generation {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, newVD.Status.Conditions)
	switch {
	case ready.Status == metav1.ConditionTrue, newVD.Status.Phase == virtv2.DiskReady, newVD.Status.Phase == virtv2.DiskLost:
		if !reflect.DeepEqual(oldVD.Spec.DataSource, newVD.Spec.DataSource) {
			return nil, errors.New("data source cannot be changed if the VirtualDisk has already been provisioned")
		}

		if !reflect.DeepEqual(oldVD.Spec.PersistentVolumeClaim.StorageClass, newVD.Spec.PersistentVolumeClaim.StorageClass) {
			return nil, errors.New("storage class cannot be changed if the VirtualDisk has already been provisioned")
		}
	case newVD.Status.Phase == virtv2.DiskTerminating:
		if !reflect.DeepEqual(oldVD.Spec, newVD.Spec) {
			return nil, errors.New("spec cannot be changed if the VirtualDisk is the process of termination")
		}
	case newVD.Status.Phase == virtv2.DiskPending:
		if newVD.Spec.PersistentVolumeClaim.StorageClass != nil && *newVD.Spec.PersistentVolumeClaim.StorageClass != "" {
			sc, err := v.scService.GetStorageClass(ctx, *newVD.Spec.PersistentVolumeClaim.StorageClass)
			if err != nil {
				return nil, err
			}
			if v.scService.IsStorageClassDeprecated(sc) {
				return nil, fmt.Errorf(
					"the provisioner of the %q storage class is deprecated; please use a different one",
					*newVD.Spec.PersistentVolumeClaim.StorageClass,
				)
			}
			if !v.scService.IsStorageClassAllowed(*newVD.Spec.PersistentVolumeClaim.StorageClass) {
				return nil, fmt.Errorf(
					"the storage class %q is not allowed; please check the module settings",
					*newVD.Spec.PersistentVolumeClaim.StorageClass,
				)
			}
		} else {
			// Check if default storage class is allowed when no storage class is specified
			mcDefaultStorageClass, err := v.scService.GetModuleStorageClass(ctx)
			if err != nil && !errors.Is(err, intsvc.ErrStorageClassNotFound) {
				return nil, fmt.Errorf("failed to fetch a default storage class from module config: %w", err)
			}

			if mcDefaultStorageClass == nil {
				defaultStorageClass, err := v.scService.GetDefaultStorageClass(ctx)
				if err != nil && !errors.Is(err, intsvc.ErrStorageClassNotFound) {
					return nil, fmt.Errorf("failed to fetch default storage class: %w", err)
				}

				if defaultStorageClass != nil && !v.scService.IsStorageClassAllowed(defaultStorageClass.Name) {
					return nil, fmt.Errorf(
						"the default storage class %q is not allowed; please check the module settings or specify a storage class name explicitly in the spec",
						defaultStorageClass.Name,
					)
				}
			}
		}
	}

	return nil, nil
}
