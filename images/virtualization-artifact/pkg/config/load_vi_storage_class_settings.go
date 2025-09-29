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

package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	// VirtualImageStorageClass is a parameter for configuring the storage class for Virtual Image on PVC.
	VirtualImageStorageClass = "VIRTUAL_IMAGE_STORAGE_CLASS"
	// VirtualImageDefaultStorageClass specifies the default storage class for virtual images on PVC when none is specified.
	VirtualImageDefaultStorageClass = "VIRTUAL_IMAGE_DEFAULT_STORAGE_CLASS"
	// VirtualImageAllowedStorageClasses is a parameter that lists all allowed storage classes for virtual images on PVC.
	VirtualImageAllowedStorageClasses = "VIRTUAL_IMAGE_ALLOWED_STORAGE_CLASSES"
)

type VirtualImageStorageClassSettings struct {
	AllowedStorageClassNames []string
	DefaultStorageClassName  string
	StorageClassName         string
}

func LoadVirtualImageStorageClassSettings() (VirtualImageStorageClassSettings, error) {
	var allowedStorageClassNames []string
	allowedStorageClassNamesRaw, exists := os.LookupEnv(VirtualImageAllowedStorageClasses)
	if exists && allowedStorageClassNamesRaw == "" {
		return VirtualImageStorageClassSettings{}, fmt.Errorf("%s is empty. Specify valid StorageClass names or remove the restriction", VirtualImageAllowedStorageClasses)
	}
	if allowedStorageClassNamesRaw != "" {
		allowedStorageClassNames = strings.Split(allowedStorageClassNamesRaw, ",")
	}

	return VirtualImageStorageClassSettings{
		AllowedStorageClassNames: allowedStorageClassNames,
		DefaultStorageClassName:  os.Getenv(VirtualImageDefaultStorageClass),
		StorageClassName:         os.Getenv(VirtualImageStorageClass),
	}, nil
}
