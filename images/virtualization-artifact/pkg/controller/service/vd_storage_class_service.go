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

type VirtualDiskStorageClassService struct {
	storageClassSettings config.VirtualDiskStorageClassSettings
}

func NewVirtualDiskStorageClassService(settings config.VirtualDiskStorageClassSettings) *VirtualDiskStorageClassService {
	return &VirtualDiskStorageClassService{
		storageClassSettings: settings,
	}
}

func (svc *VirtualDiskStorageClassService) GetStorageClass(storageClassFromSpec *string, clusterDefaultStorageClassName string) (*string, error) {
	if svc.storageClassSettings.DefaultStorageClassName == "" && len(svc.storageClassSettings.AllowedStorageClassNames) == 0 {
		return storageClassFromSpec, nil
	}

	if len(svc.storageClassSettings.AllowedStorageClassNames) > 0 && svc.storageClassSettings.DefaultStorageClassName == "" {
		if storageClassFromSpec != nil && *storageClassFromSpec != "" {
			if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, *storageClassFromSpec) {
				return storageClassFromSpec, nil
			}

			return nil, ErrStorageClassNotAvailable
		} else {
			if clusterDefaultStorageClassName == "" {
				return nil, ErrDefaultStorageClassNotFound
			}

			if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, clusterDefaultStorageClassName) {
				return &clusterDefaultStorageClassName, nil
			}

			return nil, ErrStorageClassNotAvailable
		}
	}

	if len(svc.storageClassSettings.AllowedStorageClassNames) == 0 && svc.storageClassSettings.DefaultStorageClassName != "" {
		if storageClassFromSpec == nil || *storageClassFromSpec == "" {
			return &svc.storageClassSettings.DefaultStorageClassName, nil
		}

		if *storageClassFromSpec == svc.storageClassSettings.DefaultStorageClassName {
			return storageClassFromSpec, nil
		}

		return nil, ErrStorageClassNotAvailable
	}

	if storageClassFromSpec == nil || *storageClassFromSpec == "" {
		return &svc.storageClassSettings.DefaultStorageClassName, nil
	}

	if slices.Contains(svc.storageClassSettings.AllowedStorageClassNames, *storageClassFromSpec) {
		return storageClassFromSpec, nil
	}

	return nil, ErrStorageClassNotAvailable
}
