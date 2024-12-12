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
	"os"
	"strings"
)

const (
	// VirtualDiskDefaultStorageClass specifies the default storage class for virtual disks when none is specified.
	VirtualDiskDefaultStorageClass = "VIRTUAL_DISK_DEFAULT_STORAGE_CLASS"
	// VirtualDiskAllowedStorageClasses is a parameter that lists all allowed storage classes for virtual disks.
	VirtualDiskAllowedStorageClasses = "VIRTUAL_DISK_ALLOWED_STORAGE_CLASSES"
)

type VirtualDiskStorageClassSettings struct {
	AllowedStorageClassNames []string
	DefaultStorageClassName  string
}

func LoadVirtualDiskStorageClassSettings() VirtualDiskStorageClassSettings {
	var allowedStorageClassNames []string
	allowedStorageClassNamesRaw := os.Getenv(VirtualDiskAllowedStorageClasses)
	if allowedStorageClassNamesRaw != "" {
		allowedStorageClassNames = strings.Split(allowedStorageClassNamesRaw, ",")
	}

	return VirtualDiskStorageClassSettings{
		AllowedStorageClassNames: allowedStorageClassNames,
		DefaultStorageClassName:  os.Getenv(VirtualDiskDefaultStorageClass),
	}
}
