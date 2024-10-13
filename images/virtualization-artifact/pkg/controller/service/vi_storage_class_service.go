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
	"slices"

	storev1 "k8s.io/api/storage/v1"

	"github.com/deckhouse/virtualization-controller/pkg/config"
)

type VirtualImageStorageClassService struct {
	storageClassSettings config.VirtualImageStorageClassSettings
}

func NewVirtualImageStorageClassService(settings config.VirtualImageStorageClassSettings) *VirtualImageStorageClassService {
	return &VirtualImageStorageClassService{
		storageClassSettings: settings,
	}
}

// GetStorageClass determines the storage class for VI from global settings and resource spec.
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
func (svc *VirtualImageStorageClassService) GetStorageClass(storageClassFromSpec *string, clusterDefaultStorageClass *storev1.StorageClass) (*string, error) {
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
