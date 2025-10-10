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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StorageClassMatchValidator struct {
	client    client.Client
	scService *intsvc.VirtualImageStorageClassService
}

func NewStorageClassMatchValidator(client client.Client, scService *intsvc.VirtualImageStorageClassService) *StorageClassMatchValidator {
	return &StorageClassMatchValidator{client: client, scService: scService}
}

func (v *StorageClassMatchValidator) ValidateCreate(ctx context.Context, vi *v1alpha2.VirtualImage) (admission.Warnings, error) {
	return v.Validate(ctx, vi)
}

func (v *StorageClassMatchValidator) ValidateUpdate(ctx context.Context, _, newVI *v1alpha2.VirtualImage) (admission.Warnings, error) {
	if newVI.Status.Phase == v1alpha2.ImageReady {
		return nil, nil
	}

	return v.Validate(ctx, newVI)
}

func (v *StorageClassMatchValidator) Validate(ctx context.Context, vi *v1alpha2.VirtualImage) (admission.Warnings, error) {
	if vi.Spec.Storage != v1alpha2.StoragePersistentVolumeClaim && vi.Spec.Storage != v1alpha2.StorageKubernetes {
		return nil, nil
	}

	if vi.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef || vi.Spec.DataSource.ObjectRef == nil {
		return nil, nil
	}

	if vi.Spec.DataSource.ObjectRef.Kind != v1alpha2.VirtualImageObjectRefKindVirtualDisk {
		return nil, nil
	}

	vd, err := object.FetchObject(ctx, types.NamespacedName{
		Name:      vi.Spec.DataSource.ObjectRef.Name,
		Namespace: vi.Namespace,
	}, v.client, &v1alpha2.VirtualDisk{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VirtualDisk %q: %w", vi.Spec.DataSource.ObjectRef.Name, err)
	}

	if vd == nil {
		return nil, nil
	}

	defaultStorageClass, err := v.scService.GetDefaultStorageClass(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get default storage class: %w", err)
	}

	var viStorageClass string
	if vi.Spec.PersistentVolumeClaim.StorageClass != nil && *vi.Spec.PersistentVolumeClaim.StorageClass != "" {
		viStorageClass = *vi.Spec.PersistentVolumeClaim.StorageClass
	} else {
		viStorageClass = defaultStorageClass.Name
	}

	var vdStorageClass string
	switch {
	case vd.Status.StorageClassName != "":
		vdStorageClass = vd.Status.StorageClassName
	case vd.Spec.PersistentVolumeClaim.StorageClass != nil && *vd.Spec.PersistentVolumeClaim.StorageClass != "":
		vdStorageClass = *vd.Spec.PersistentVolumeClaim.StorageClass
	default:
		vdStorageClass = defaultStorageClass.Name // if VD only created and not ready yet, it will use default StorageClass
	}

	if viStorageClass != vdStorageClass {
		return nil, fmt.Errorf(
			"cannot create VirtualImage from VirtualDisk %q with different storage classes: "+
				"VirtualDisk uses %q, VirtualImage specifies %q",
			vd.Name, vdStorageClass, viStorageClass,
		)
	}

	return nil, nil
}
