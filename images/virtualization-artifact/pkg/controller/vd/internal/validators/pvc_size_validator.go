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
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PVCSizeValidator struct {
	client client.Client
}

func NewPVCSizeValidator(client client.Client) *PVCSizeValidator {
	return &PVCSizeValidator{client: client}
}

func (v *PVCSizeValidator) ValidateCreate(ctx context.Context, vd *virtv2.VirtualDisk) (admission.Warnings, error) {
	if vd.Spec.PersistentVolumeClaim.Size != nil && vd.Spec.PersistentVolumeClaim.Size.IsZero() {
		return nil, fmt.Errorf("virtual disk size must be greater than 0")
	}

	if vd.Spec.DataSource == nil && vd.Spec.PersistentVolumeClaim.Size == nil {
		return nil, fmt.Errorf("if the data source is not specified, it's necessary to set spec.PersistentVolumeClaim.size to create blank virtual disk")
	}

	if vd.Spec.DataSource == nil || vd.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || vd.Spec.DataSource.ObjectRef == nil {
		return nil, nil
	}

	var unpackedBytes string

	switch vd.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualDiskObjectRefKindVirtualImage,
		virtv2.VirtualDiskObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, vd.Spec.DataSource, vd, v.client)
		if err != nil {
			return nil, err
		}

		if !dvcrDataSource.IsReady() {
			return nil, nil
		}

		unpackedBytes = dvcrDataSource.GetSize().UnpackedBytes

	// TODO validate for snapshot kind also.
	default:
		return nil, nil
	}

	unpackedSize, err := resource.ParseQuantity(unpackedBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse unpacked bytes %s: %w", unpackedBytes, err)
	}

	_, err = service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *PVCSizeValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *virtv2.VirtualDisk) (admission.Warnings, error) {
	if oldVD.Spec.PersistentVolumeClaim.Size == newVD.Spec.PersistentVolumeClaim.Size {
		return nil, nil
	}

	if newVD.Spec.PersistentVolumeClaim.Size == nil {
		return nil, errors.New("spec.persistentVolumeClaim.size cannot be omitted once set")
	}

	if newVD.Spec.PersistentVolumeClaim.Size.IsZero() {
		return nil, fmt.Errorf("virtual disk size must be greater than 0")
	}

	if oldVD.Spec.PersistentVolumeClaim.Size != nil && newVD.Spec.PersistentVolumeClaim.Size.Cmp(*oldVD.Spec.PersistentVolumeClaim.Size) == -1 {
		return nil, fmt.Errorf(
			"spec.persistentVolumeClaim.size value (%s) should be greater than or equal to the current value (%s)",
			newVD.Spec.PersistentVolumeClaim.Size.String(),
			oldVD.Spec.PersistentVolumeClaim.Size.String(),
		)
	}

	if newVD.Spec.DataSource == nil || newVD.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || newVD.Spec.DataSource.ObjectRef == nil {
		return nil, nil
	}

	var unpackedBytes string

	switch newVD.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualDiskObjectRefKindVirtualImage,
		virtv2.VirtualDiskObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, newVD.Spec.DataSource, newVD, v.client)
		if err != nil {
			return nil, err
		}

		if !dvcrDataSource.IsReady() {
			return nil, nil
		}

		unpackedBytes = dvcrDataSource.GetSize().UnpackedBytes

	// TODO validate for snapshot kind also.
	default:
		return nil, nil
	}

	unpackedSize, err := resource.ParseQuantity(unpackedBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse unpacked bytes %s: %w", unpackedBytes, err)
	}

	_, err = service.GetValidatedPVCSize(newVD.Spec.PersistentVolumeClaim.Size, unpackedSize)
	if err != nil {
		return nil, err
	}

	return nil, nil
}
