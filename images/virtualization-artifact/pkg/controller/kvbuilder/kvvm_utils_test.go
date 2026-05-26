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

package kvbuilder

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
)

var _ = Describe("cleanupRemovedStaticDisks", func() {
	const (
		vmName      = "test-vm"
		vmNamespace = "test-ns"

		oldDisk1Name = "vd-old-disk-1"
		oldDisk2Name = "vd-old-disk-2"
		oldPVC1Name  = "pvc-old-disk-1"
		oldPVC2Name  = "pvc-old-disk-2"

		newDisk1Name = "vd-new-disk-1"
		newDisk2Name = "vd-new-disk-2"
	)

	var kvvm *KVVM

	BeforeEach(func() {
		kvvm = NewEmptyKVVM(
			namespacedName(vmName, vmNamespace),
			KVVMOptions{},
		)
		// Add initial volumes to KVVM
		kvvm.Resource.Spec.Template.Spec.Volumes = []virtv1.Volume{
			{
				Name: oldDisk1Name,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: oldPVC1Name,
						},
						Hotpluggable: true,
					},
				},
			},
			{
				Name: oldDisk2Name,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: oldPVC2Name,
						},
						Hotpluggable: false,
					},
				},
			},
		}
		kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks = []virtv1.Disk{
			{Name: oldDisk1Name},
			{Name: oldDisk2Name},
		}
	})

	Describe("when VM is stopped (isVmRunning=false)", func() {
		It("should remove all disks that are not in VM spec, regardless of hotpluggable flag", func() {
			specDiskNames := map[string]struct{}{
				newDisk1Name: {},
				newDisk2Name: {},
			}
			hotpluggableVolumes := map[string]struct{}{
				oldDisk1Name: {}, // hotpluggable
			}

			cleanupRemovedStaticDisks(kvvm, specDiskNames, hotpluggableVolumes, false)

			// Should remove old-disk-1 (hotpluggable) and old-disk-2 (non-hotpluggable)
			// because VM is stopped
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(0))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(0))
		})
	})

	Describe("when VM is running (isVmRunning=true)", func() {
		It("should only remove non-hotpluggable disks that are not in VM spec", func() {
			specDiskNames := map[string]struct{}{
				newDisk1Name: {},
				newDisk2Name: {},
			}
			hotpluggableVolumes := map[string]struct{}{
				oldDisk1Name: {}, // hotpluggable - should NOT be removed
			}

			cleanupRemovedStaticDisks(kvvm, specDiskNames, hotpluggableVolumes, true)

			// Should only remove old-disk-2 (non-hotpluggable)
			// old-disk-1 should stay because it's hotpluggable
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].Name).To(Equal(oldDisk1Name))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks[0].Name).To(Equal(oldDisk1Name))
		})
	})
})

func namespacedName(name, namespace string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: namespace}
}
