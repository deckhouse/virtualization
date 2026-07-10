/*
Copyright 2026 Flant JSC

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("StorageFormat", func() {
	It("returns raw for block PVCs", func() {
		pvc := &corev1.PersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeMode: ptr.To(corev1.PersistentVolumeBlock),
			},
		}
		Expect(StorageFormat(pvc)).To(Equal(FormatRAW))
	})

	It("returns qcow2 for filesystem PVCs", func() {
		pvc := &corev1.PersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeMode: ptr.To(corev1.PersistentVolumeFilesystem),
			},
		}
		Expect(StorageFormat(pvc)).To(Equal(FormatQCOW2))
	})

	It("returns qcow2 when volume mode is unset", func() {
		Expect(StorageFormat(&corev1.PersistentVolumeClaim{})).To(Equal(FormatQCOW2))
		Expect(StorageFormat(nil)).To(Equal(FormatQCOW2))
	})
})
