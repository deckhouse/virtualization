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

package config

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ImportSettings struct {
	ImporterImage string
	UploaderImage string
	BounderImage  string
	Requirements  corev1.ResourceRequirements
}

// TODO(future) live migration settings will be here. Now just a place for the default policy.
const (
	DefaultLiveMigrationPolicy = v1alpha2.PreferSafeMigrationPolicy
)

type VirtualDiskStorageClassSettings struct {
	AllowedStorageClassNames []string
	DefaultStorageClassName  string
}

type VirtualImageStorageClassSettings struct {
	AllowedStorageClassNames []string
	DefaultStorageClassName  string
	StorageClassName         string
}
