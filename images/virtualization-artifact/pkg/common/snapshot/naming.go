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

package snapshot

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
)

const (
	vmSnapshotSecretPrefix         = "d8v-vms-"
	vdSnapshotVolumeSnapshotPrefix = "d8v-vds-"

	maxResourceNameLength = 253
	uuidLength            = 36
)

func GetVMSnapshotSecretName(name string, uid types.UID) string {
	maxNameLength := maxResourceNameLength - len(vmSnapshotSecretPrefix) - 1 - uuidLength
	truncatedName := truncateName(name, maxNameLength)
	return fmt.Sprintf("%s%s-%s", vmSnapshotSecretPrefix, truncatedName, uid)
}

func GetLegacyVMSnapshotSecretName(name string) string {
	return name
}

func GetVDSnapshotVolumeSnapshotName(name string, uid types.UID) string {
	maxNameLength := maxResourceNameLength - len(vdSnapshotVolumeSnapshotPrefix) - 1 - uuidLength
	truncatedName := truncateName(name, maxNameLength)
	return fmt.Sprintf("%s%s-%s", vdSnapshotVolumeSnapshotPrefix, truncatedName, uid)
}

func GetLegacyVDSnapshotVolumeSnapshotName(name string) string {
	return name
}

func truncateName(name string, maxLength int) string {
	if len(name) <= maxLength {
		return name
	}
	return name[:maxLength]
}
