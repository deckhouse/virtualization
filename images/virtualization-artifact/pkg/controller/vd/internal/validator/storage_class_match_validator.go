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
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StorageClassMatchValidator struct {
	client    client.Client
	scService *intsvc.VirtualDiskStorageClassService
}

func NewStorageClassMatchValidator(client client.Client, scService *intsvc.VirtualDiskStorageClassService) *StorageClassMatchValidator {
	return &StorageClassMatchValidator{client: client, scService: scService}
}

func (v *StorageClassMatchValidator) ValidateCreate(ctx context.Context, vd *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	return v.Validate(ctx, vd)
}

func (v *StorageClassMatchValidator) ValidateUpdate(ctx context.Context, _, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if newVD.Status.Phase == v1alpha2.DiskReady {
		return nil, nil
	}

	return v.Validate(ctx, newVD)
}

func (v *StorageClassMatchValidator) Validate(ctx context.Context, vd *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef || vd.Spec.DataSource.ObjectRef == nil {
		return nil, nil
	}

	if vd.Spec.DataSource.ObjectRef.Kind != v1alpha2.VirtualDiskObjectRefKindVirtualImage {
		return nil, nil
	}

	vi, err := object.FetchObject(ctx, types.NamespacedName{
		Name:      vd.Spec.DataSource.ObjectRef.Name,
		Namespace: vd.Namespace,
	}, v.client, &v1alpha2.VirtualImage{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VirtualImage %q: %w", vd.Spec.DataSource.ObjectRef.Name, err)
	}

	if vi == nil {
		return nil, nil
	}

	if vi.Spec.Storage != v1alpha2.StoragePersistentVolumeClaim && vi.Spec.Storage != v1alpha2.StorageKubernetes {
		return nil, nil
	}

	defaultStorageClass, err := v.scService.GetDefaultStorageClass(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get default storage class: %w", err)
	}

	var vdStorageClass string
	if vd.Spec.PersistentVolumeClaim.StorageClass != nil && *vd.Spec.PersistentVolumeClaim.StorageClass != "" {
		vdStorageClass = *vd.Spec.PersistentVolumeClaim.StorageClass
	} else {
		vdStorageClass = defaultStorageClass.Name
	}

	var viStorageClass string
	switch {
	case vi.Status.StorageClassName != "":
		viStorageClass = vi.Status.StorageClassName
	case vi.Spec.PersistentVolumeClaim.StorageClass != nil && *vi.Spec.PersistentVolumeClaim.StorageClass != "":
		viStorageClass = *vi.Spec.PersistentVolumeClaim.StorageClass
	default:
		viStorageClass = defaultStorageClass.Name // if VI only created and not ready yet, it will use default StorageClass
	}

	if vdStorageClass != viStorageClass {
		return nil, fmt.Errorf(
			"cannot create VirtualDisk from VirtualImage %q with different storage classes: "+
				"VirtualImage uses %q, VirtualDisk specifies %q",
			vi.Name, viStorageClass, vdStorageClass,
		)
	}

	return nil, nil
}
