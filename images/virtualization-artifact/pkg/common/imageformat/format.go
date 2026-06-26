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

package imageformat

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	FormatISO   = "iso"
	FormatRAW   = "raw"
	FormatQCOW2 = "qcow2"
)

func IsISO(format string) bool {
	return strings.ToLower(format) == FormatISO
}

// StorageFormat returns the image format actually stored on a PVC.
// Block volumes are populated as a flat raw disk; filesystem volumes keep a qcow2 file.
func StorageFormat(pvc *corev1.PersistentVolumeClaim) string {
	if pvc != nil && pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode == corev1.PersistentVolumeBlock {
		return FormatRAW
	}
	return FormatQCOW2
}
