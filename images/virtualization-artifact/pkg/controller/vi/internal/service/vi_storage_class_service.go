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

package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	storev1 "k8s.io/api/storage/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
)

var (
	ErrStorageClassNotFound   = errors.New("storage class not found")
	ErrStorageClassNotAllowed = errors.New("storage class not allowed")
)

type VirtualImageStorageClassService struct {
	*service.BaseStorageClassService

	storageClassSettings config.VirtualImageStorageClassSettings
}

func NewVirtualImageStorageClassService(svc *service.BaseStorageClassService, settings config.VirtualImageStorageClassSettings) *VirtualImageStorageClassService {
	return &VirtualImageStorageClassService{
		BaseStorageClassService: svc,
		storageClassSettings:    settings,
	}
}

// GetValidatedStorageClass determines the storage class for VI from global settings and resource spec.
//
// Global settings contain a default storage class and an array of allowed storageClasses from the ModuleConfig.
// Storage class is allowed if contained in the "allowed" array.
//
// Storage class from the spec has the most priority with fallback to global settings:
// 1. Return `storageClassName` if specified in the resource spec and allowed by global settings.
// 2. Return global `defaultStorageClass` if specified.
// 3. Return cluster-wide default storage class if specified and allowed.
//
// Errors:
// 1. Return error if no storage class is specified.
// 2. Return error if specified non-empty class is not allowed.
func (svc *VirtualImageStorageClassService) GetValidatedStorageClass(storageClassFromSpec *string, clusterDefaultStorageClass *storev1.StorageClass) (*string, error) {
	if svc.storageClassSettings.DefaultStorageClassName == "" && len(svc.storageClassSettings.AllowedStorageClassNames) == 0 {
		if svc.storageClassSettings.StorageClassName == "" {
			return storageClassFromSpec, nil
		}

		if storageClassFromSpec == nil || *storageClassFromSpec == "" || *storageClassFromSpec == svc.storageClassSettings.StorageClassName {
			return &svc.storageClassSettings.StorageClassName, nil
		}

		return nil, ErrStorageClassNotAllowed
	}

	if storageClassFromSpec != nil && *storageClassFromSpec != "" {
		if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, *storageClassFromSpec) {
			return storageClassFromSpec, nil
		}

		if svc.storageClassSettings.DefaultStorageClassName != "" && svc.storageClassSettings.DefaultStorageClassName == *storageClassFromSpec {
			return storageClassFromSpec, nil
		}

		return nil, ErrStorageClassNotAllowed
	}

	if svc.storageClassSettings.DefaultStorageClassName != "" {
		return &svc.storageClassSettings.DefaultStorageClassName, nil
	}

	if clusterDefaultStorageClass != nil && clusterDefaultStorageClass.Name != "" {
		if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, clusterDefaultStorageClass.Name) {
			return &clusterDefaultStorageClass.Name, nil
		}

		return nil, ErrStorageClassNotAllowed
	}

	return nil, ErrStorageClassNotFound
}

func (svc *VirtualImageStorageClassService) IsStorageClassAllowed(scName string) bool {
	if svc.storageClassSettings.DefaultStorageClassName == "" && len(svc.storageClassSettings.AllowedStorageClassNames) == 0 {
		return true
	}

	if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, scName) {
		return true
	}

	if svc.storageClassSettings.DefaultStorageClassName != "" && svc.storageClassSettings.DefaultStorageClassName == scName {
		return true
	}

	return false
}

func (svc *VirtualImageStorageClassService) GetModuleStorageClass(ctx context.Context) (*storev1.StorageClass, error) {
	return svc.GetStorageClass(ctx, svc.storageClassSettings.DefaultStorageClassName)
}

func (svc *VirtualImageStorageClassService) ValidateClaimPropertySets(sp *cdiv1.StorageProfile) error {
	if sp == nil {
		return fmt.Errorf("the storage profile cannot be nil; please report a bug")
	}

	for _, cps := range sp.Status.ClaimPropertySets {
		if slices.Contains(cps.AccessModes, corev1.ReadWriteMany) && *cps.VolumeMode == corev1.PersistentVolumeBlock {
			return nil
		}
	}

	return fmt.Errorf(
		"the storage class %q lacks of capabilities to support 'Virtual Images on PVC' function; use StorageClass that supports volume mode 'Block' and access mode 'ReadWriteMany'",
		sp.Name,
	)
}
