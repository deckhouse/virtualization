/*
Copyright 2025 Flant JSC

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

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/supervd/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/version"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type volumeAndAccessModesGetter interface {
	GetVolumeAndAccessModes(ctx context.Context, obj client.Object, sc *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error)
}
type StorageClassValidator struct {
	client     client.Client
	scService  *intsvc.VirtualDiskStorageClassService
	modeGetter volumeAndAccessModesGetter
}

func NewMigrationStorageClassValidator(client client.Client, scService *intsvc.VirtualDiskStorageClassService, modeGetter volumeAndAccessModesGetter) *StorageClassValidator {
	return &StorageClassValidator{
		client:     client,
		scService:  scService,
		modeGetter: modeGetter,
	}
}

func (v *StorageClassValidator) ValidateCreate(ctx context.Context, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
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
	}

	return nil, nil
}

func (v *StorageClassValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if oldVD.Generation == newVD.Generation {
		return nil, nil
	}

	if newVD.Status.Phase == v1alpha2.DiskPending {
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
		}
	}

	if err := v.validateTargetStorageClassForVolumeMigration(ctx, newVD, oldVD); err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *StorageClassValidator) validateTargetStorageClassForVolumeMigration(ctx context.Context, newVD, oldVD *v1alpha2.VirtualDisk) error {
	if equality.Semantic.DeepEqual(oldVD.Spec.PersistentVolumeClaim.StorageClass, newVD.Spec.PersistentVolumeClaim.StorageClass) {
		return nil
	}

	// For run volume migration storage class must be specified in the spec.
	// If storage class is nil, migration is canceled or not requested.
	if newVD.Spec.PersistentVolumeClaim.StorageClass == nil {
		return nil
	}

	if !commonvd.VolumeMigrationEnabled(featuregates.Default(), newVD) {
		return fmt.Errorf("storage class cannot be changed if volume migration is not enabled (EDITION=%q)", version.GetEdition())
	}

	vmName := commonvd.GetCurrentlyMountedVMName(newVD)
	if vmName == "" {
		return fmt.Errorf("storage class cannot be changed if the VirtualDisk not mounted to virtual machine")
	}

	vm := &v1alpha2.VirtualMachine{}
	err := v.client.Get(ctx, client.ObjectKey{Name: vmName, Namespace: newVD.Namespace}, vm)
	if err != nil {
		return err
	}

	if !(vm.Status.Phase == v1alpha2.MachineRunning || vm.Status.Phase == v1alpha2.MachineMigrating) {
		return fmt.Errorf("storage class cannot be changed unless the VirtualDisk is mounted to a running virtual machine")
	}

	for _, bd := range vm.Status.BlockDeviceRefs {
		if bd.Hotplugged {
			return fmt.Errorf("for now, changing the storage class is not allowed if the virtual machine has hot-plugged block devices")
		}
	}

	currentStorageClassName := newVD.Status.StorageClassName
	currentStorageClass, err := v.scService.GetStorageClass(ctx, currentStorageClassName)
	if err != nil {
		return err
	}
	currentMode, _, err := v.modeGetter.GetVolumeAndAccessModes(ctx, newVD, currentStorageClass)
	if err != nil {
		return err
	}

	desiredStorageClassName := *newVD.Spec.PersistentVolumeClaim.StorageClass
	desiredStorageClass, err := v.scService.GetStorageClass(ctx, desiredStorageClassName)
	if err != nil {
		return err
	}
	desiredMode, _, err := v.modeGetter.GetVolumeAndAccessModes(ctx, newVD, desiredStorageClass)
	if err != nil {
		return err
	}

	// NOTE: Volume mode migration is currently not supported. This is a known limitation and may be addressed in future releases.
	if currentMode != desiredMode {
		return fmt.Errorf("changing storage class is not allowed because migration to a different volume mode is not supported yet. Please use a storage class with the same volume mode, or contact support for future migration plans")
	}

	return nil
}
