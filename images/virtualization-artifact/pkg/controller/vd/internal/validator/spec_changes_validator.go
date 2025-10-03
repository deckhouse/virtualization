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

	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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

func (v *SpecChangesValidator) ValidateCreate(ctx context.Context, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
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

func (v *SpecChangesValidator) ValidateUpdate(ctx context.Context, oldVD, newVD *v1alpha2.VirtualDisk) (admission.Warnings, error) {
	if oldVD.Generation == newVD.Generation {
		return nil, nil
	}

	ready, _ := conditions.GetCondition(vdcondition.ReadyType, newVD.Status.Conditions)
	switch {
	case ready.Status == metav1.ConditionTrue, newVD.Status.Phase == v1alpha2.DiskReady, newVD.Status.Phase == v1alpha2.DiskLost:
		if !reflect.DeepEqual(oldVD.Spec.DataSource, newVD.Spec.DataSource) {
			return nil, errors.New("data source cannot be changed if the VirtualDisk has already been provisioned")
		}

		if !reflect.DeepEqual(oldVD.Spec.PersistentVolumeClaim.StorageClass, newVD.Spec.PersistentVolumeClaim.StorageClass) {
			if commonvd.VolumeMigrationEnabled(featuregates.Default(), newVD) {
				vmName := commonvd.GetCurrentlyMountedVMName(newVD)
				if vmName == "" {
					return nil, errors.New("storage class cannot be changed if the VirtualDisk not mounted to virtual machine")
				}

				vm := &v1alpha2.VirtualMachine{}
				err := v.client.Get(ctx, client.ObjectKey{Name: vmName, Namespace: newVD.Namespace}, vm)
				if err != nil {
					return nil, err
				}

				if !(vm.Status.Phase == v1alpha2.MachineRunning || vm.Status.Phase == v1alpha2.MachineMigrating) {
					return nil, errors.New("storage class cannot be changed unless the VirtualDisk is mounted to a running virtual machine")
				}

				for _, bd := range vm.Status.BlockDeviceRefs {
					if bd.Hotplugged {
						return nil, errors.New("for now, changing the storage class is not allowed if the virtual machine has hot-plugged block devices")
					}
				}
			} else {
				return nil, errors.New("storage class cannot be changed if the VirtualDisk has already been provisioned")
			}
		}
	case newVD.Status.Phase == v1alpha2.DiskTerminating:
		if !reflect.DeepEqual(oldVD.Spec, newVD.Spec) {
			return nil, errors.New("spec cannot be changed if the VirtualDisk is the process of termination")
		}
	case newVD.Status.Phase == v1alpha2.DiskPending:
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

	return nil, nil
}
