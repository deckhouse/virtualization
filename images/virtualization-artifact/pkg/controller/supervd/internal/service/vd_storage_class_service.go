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
	"slices"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

var (
	ErrStorageClassNotFound   = errors.New("storage class not found")
	ErrStorageClassNotAllowed = errors.New("storage class not allowed")
)

type VirtualDiskStorageClassService struct {
	*service.BaseStorageClassService

	storageClassSettings config.VirtualDiskStorageClassSettings
}

func NewVirtualDiskStorageClassService(svc *service.BaseStorageClassService, settings config.VirtualDiskStorageClassSettings) *VirtualDiskStorageClassService {
	return &VirtualDiskStorageClassService{
		BaseStorageClassService: svc,
		storageClassSettings:    settings,
	}
}

// GetValidatedStorageClass determines the storage class for VD from global settings and resource spec.
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
func (svc *VirtualDiskStorageClassService) GetValidatedStorageClass(storageClassFromSpec *string, clusterDefaultStorageClass *storagev1.StorageClass) (*string, error) {
	if svc.storageClassSettings.DefaultStorageClassName == "" && len(svc.storageClassSettings.AllowedStorageClassNames) == 0 {
		return storageClassFromSpec, nil
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

func (svc *VirtualDiskStorageClassService) IsStorageClassAllowed(scName string) bool {
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

func (svc *VirtualDiskStorageClassService) GetModuleStorageClass(ctx context.Context) (*storagev1.StorageClass, error) {
	return svc.GetStorageClass(ctx, svc.storageClassSettings.DefaultStorageClassName)
}

func (svc *VirtualDiskStorageClassService) GetPersistentVolumeClaim(ctx context.Context, sup supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
	return svc.BaseStorageClassService.GetPersistentVolumeClaim(ctx, sup)
}
