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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type PVCSizeValidator struct {
	client client.Client
}

func NewPVCSizeValidator(client client.Client) *PVCSizeValidator {
	return &PVCSizeValidator{client: client}
}

func (v *PVCSizeValidator) ValidateCreate(ctx context.Context, vd *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if vd.Spec.PersistentVolumeClaim.Size != nil && vd.Spec.PersistentVolumeClaim.Size.IsZero() {
		return nil, fmt.Errorf("virtual disk size must be greater than 0")
	}

	if vd.Spec.DataSource == nil && vd.Spec.PersistentVolumeClaim.Size == nil {
		return nil, fmt.Errorf("if the data source is not specified, it's necessary to set spec.PersistentVolumeClaim.size to create blank virtual disk")
	}

	if vd.Spec.DataSource == nil || vd.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef || vd.Spec.DataSource.ObjectRef == nil {
		return nil, nil
	}

	var unpackedSize resource.Quantity

	switch vd.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualDiskObjectRefKindVirtualImage,
		v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, vd.Spec.DataSource, vd, v.client)
		if err != nil {
			return nil, err
		}

		if !dvcrDataSource.IsReady() {
			return nil, nil
		}

		unpackedSize, err = resource.ParseQuantity(dvcrDataSource.GetSize().UnpackedBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse unpacked bytes %s: %w", unpackedSize.String(), err)
		}

	case v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot:
		vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{
			Name:      vd.Spec.DataSource.ObjectRef.Name,
			Namespace: vd.Namespace,
		}, v.client, &v1alpha2.VirtualDiskSnapshot{})
		if err != nil {
			return nil, err
		}

		if vdSnapshot == nil || vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
			return nil, nil
		}

		vs, err := object.FetchObject(ctx, types.NamespacedName{
			Name:      vdSnapshot.Status.VolumeSnapshotName,
			Namespace: vdSnapshot.Namespace,
		}, v.client, &vsv1.VolumeSnapshot{})
		if err != nil {
			return nil, err
		}

		if vs == nil || vs.Status == nil || vs.Status.RestoreSize == nil {
			return nil, nil
		}

		unpackedSize = *vs.Status.RestoreSize
	default:
		return nil, nil
	}

	_, err := service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, service.ErrInsufficientPVCSize):
		return admission.Warnings{
			service.CapitalizeFirstLetter(err.Error()),
		}, nil
	default:
		return nil, err
	}
}

func (v *PVCSizeValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	sizeEqual := equality.Semantic.DeepEqual(oldVD.Spec.PersistentVolumeClaim.Size, newVD.Spec.PersistentVolumeClaim.Size)
	if oldVD.Status.Phase == v1alpha2.DiskMigrating && !sizeEqual {
		return nil, errors.New("spec.persistentVolumeClaim.size cannot be changed during migration. Please wait for the migration to finish")
	}

	if sizeEqual {
		return nil, nil
	}
	var (
		oldSize resource.Quantity
		newSize resource.Quantity
	)

	if s := oldVD.Spec.PersistentVolumeClaim.Size; s != nil {
		oldSize = *s
	}

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, newVD.Status.Conditions)
	if s := newVD.Spec.PersistentVolumeClaim.Size; s != nil {
		newSize = *s
	} else if ready.Status == metav1.ConditionTrue ||
		newVD.Status.Phase != v1alpha2.DiskPending &&
			newVD.Status.Phase != v1alpha2.DiskProvisioning &&
			newVD.Status.Phase != v1alpha2.DiskWaitForFirstConsumer {
		return nil, errors.New("spec.persistentVolumeClaim.size cannot be omitted once set")
	}

	if ready.Status == metav1.ConditionTrue ||
		newVD.Status.Phase != v1alpha2.DiskPending &&
			newVD.Status.Phase != v1alpha2.DiskProvisioning &&
			newVD.Status.Phase != v1alpha2.DiskWaitForFirstConsumer {
		if newSize.Cmp(oldSize) == common.CmpLesser {
			return nil, fmt.Errorf(
				"spec.persistentVolumeClaim.size value (%s) should be greater than or equal to the current value (%s)",
				newSize.String(),
				oldSize.String(),
			)
		}
	}

	if newVD.Spec.DataSource == nil || newVD.Spec.DataSource.Type != v1alpha2.DataSourceTypeObjectRef || newVD.Spec.DataSource.ObjectRef == nil {
		return nil, nil
	}

	var unpackedSize resource.Quantity

	switch newVD.Spec.DataSource.ObjectRef.Kind {
	case v1alpha2.VirtualDiskObjectRefKindVirtualImage,
		v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage:
		dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, newVD.Spec.DataSource, newVD, v.client)
		if err != nil {
			return nil, err
		}

		if !dvcrDataSource.IsReady() {
			return nil, nil
		}

		unpackedSize, err = resource.ParseQuantity(dvcrDataSource.GetSize().UnpackedBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse unpacked bytes %s: %w", unpackedSize.String(), err)
		}

	case v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot:
		vdSnapshot, err := object.FetchObject(ctx, types.NamespacedName{
			Name:      newVD.Spec.DataSource.ObjectRef.Name,
			Namespace: newVD.Namespace,
		}, v.client, &v1alpha2.VirtualDiskSnapshot{})
		if err != nil {
			return nil, err
		}

		if vdSnapshot == nil || vdSnapshot.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
			return nil, nil
		}

		vs, err := object.FetchObject(ctx, types.NamespacedName{
			Name:      vdSnapshot.Status.VolumeSnapshotName,
			Namespace: vdSnapshot.Namespace,
		}, v.client, &vsv1.VolumeSnapshot{})
		if err != nil {
			return nil, err
		}

		if vs == nil || vs.Status == nil || vs.Status.RestoreSize == nil {
			return nil, nil
		}

		unpackedSize = *vs.Status.RestoreSize

	default:
		return nil, nil
	}

	_, err := service.GetValidatedPVCSize(&newSize, unpackedSize)
	switch {
	case err == nil:
		return nil, nil
	case errors.Is(err, service.ErrInsufficientPVCSize):
		return admission.Warnings{
			service.CapitalizeFirstLetter(err.Error()),
		}, nil
	default:
		return nil, err
	}
}
