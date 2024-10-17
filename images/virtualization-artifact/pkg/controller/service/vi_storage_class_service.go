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

func (svc *VirtualImageStorageClassService) GetStorageClass(storageClassFromSpec, clusterDefaultStorageClassName string) (string, error) {
	// if settings is empty
	if svc.storageClassSettings.DefaultStorageClassName == "" && len(svc.storageClassSettings.AllowedStorageClassNames) == 0 {
		if svc.storageClassSettings.StorageClassName == "" {
			if clusterDefaultStorageClassName == "" {
				return "", ErrDefaultStorageClassNotFound
			}

			if storageClassFromSpec == "" || storageClassFromSpec == clusterDefaultStorageClassName {
				return clusterDefaultStorageClassName, nil
			}

			return "", ErrStorageClassNotAvailable
		}

		if storageClassFromSpec == "" || storageClassFromSpec == svc.storageClassSettings.StorageClassName {
			return svc.storageClassSettings.StorageClassName, nil
		}

		return "", ErrStorageClassNotAvailable
	}

	// if AllowedStorageClassNames is existed, but DefaultStorageClassName is empty
	if len(svc.storageClassSettings.AllowedStorageClassNames) > 0 && svc.storageClassSettings.DefaultStorageClassName == "" {
		if storageClassFromSpec != "" {
			if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, storageClassFromSpec) {
				return storageClassFromSpec, nil
			}

			return "", ErrStorageClassNotAvailable
		} else {
			if clusterDefaultStorageClassName == "" {
				return "", ErrDefaultStorageClassNotFound
			}

			if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, clusterDefaultStorageClassName) {
				return clusterDefaultStorageClassName, nil
			}

			return "", ErrStorageClassNotAvailable
		}
	}

	// if AllowedStorageClassNames is empty, but DefaultStorageClassName exist
	if len(svc.storageClassSettings.AllowedStorageClassNames) == 0 && svc.storageClassSettings.DefaultStorageClassName != "" {
		if storageClassFromSpec == "" {
			return svc.storageClassSettings.DefaultStorageClassName, nil
		}

		if storageClassFromSpec == svc.storageClassSettings.DefaultStorageClassName {
			return storageClassFromSpec, nil
		}

		return "", ErrStorageClassNotAvailable
	}

	if storageClassFromSpec == "" {
		return svc.storageClassSettings.DefaultStorageClassName, nil
	}

	if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, storageClassFromSpec) {
		return storageClassFromSpec, nil
	}

	return "", ErrStorageClassNotAvailable
}
