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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func newKVVMWithVMBDAVolume(pvcName string) *KVVM {
	const (
		vmNamespace = "test-ns"
		diskName    = "data-disk"
	)

	kvvm := NewEmptyKVVM(
		namespacedName("test-vm", vmNamespace),
		KVVMOptions{},
	)
	kvvm.Resource.Spec.Template.Spec.Volumes = []virtv1.Volume{
		{
			Name: GenerateVDDiskName(diskName),
			VolumeSource: virtv1.VolumeSource{
				PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
					Hotpluggable: true,
				},
			},
		},
	}
	kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks = []virtv1.Disk{
		{Name: GenerateVDDiskName(diskName)},
	}
	return kvvm
}

var _ = Describe("syncAttachedVMBDAHotplugVolumes", func() {
	const (
		vmName      = "test-vm"
		vmNamespace = "test-ns"
		diskName    = "data-disk"
		sourcePVC   = "pvc-source"
		targetPVC   = "pvc-target"
	)

	It("should switch existing VMBDA volume back to source PVC after migration rollback", func() {
		kvvm := newKVVMWithVMBDAVolume(targetPVC)
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:       diskName,
				Namespace:  vmNamespace,
				UID:        "vd-uid",
				Generation: 2,
			},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: sourcePVC},
				Conditions: []metav1.Condition{{
					Type:               vdcondition.MigratingType.String(),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: 2,
					Reason:             "MigrationReverted",
				}},
				MigrationState: v1alpha2.VirtualDiskMigrationState{
					SourcePVC:      sourcePVC,
					TargetPVC:      targetPVC,
					StartTimestamp: metav1.Now(),
					EndTimestamp:   metav1.Now(),
				},
			},
		}

		err := syncAttachedVMBDAHotplugVolumes(
			kvvm,
			map[string]*v1alpha2.VirtualDisk{diskName: vd},
			nil,
			nil,
			map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment{
				{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: diskName}: nil,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim).NotTo(BeNil())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(sourcePVC))
	})

	It("should remove terminating VirtualDisk attached via VMBDA", func() {
		kvvm := newKVVMWithVMBDAVolume(sourcePVC)
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: diskName, Namespace: vmNamespace},
			Status: v1alpha2.VirtualDiskStatus{
				Phase:  v1alpha2.DiskTerminating,
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: sourcePVC},
			},
		}

		err := syncAttachedVMBDAHotplugVolumes(
			kvvm,
			map[string]*v1alpha2.VirtualDisk{diskName: vd},
			nil,
			nil,
			map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment{
				{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: diskName}: nil,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(BeEmpty())
		Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(BeEmpty())
	})

	It("should remove missing VirtualDisk attached via VMBDA", func() {
		kvvm := newKVVMWithVMBDAVolume(sourcePVC)

		err := syncAttachedVMBDAHotplugVolumes(
			kvvm,
			map[string]*v1alpha2.VirtualDisk{},
			nil,
			nil,
			map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment{
				{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: diskName}: nil,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(BeEmpty())
		Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(BeEmpty())
	})

	It("should keep VMBDA ClusterVirtualImage volume when the image is not resolved yet", func() {
		volName := GenerateCVIDiskName("cvi-hotplug")
		kvvm := newKVVMWithVMBDAImageVolume(volName)

		err := syncAttachedVMBDAHotplugVolumes(
			kvvm,
			nil,
			nil,
			map[string]*v1alpha2.ClusterVirtualImage{},
			map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment{
				{Kind: v1alpha2.VMBDAObjectRefKindClusterVirtualImage, Name: "cvi-hotplug"}: nil,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].Name).To(Equal(volName))
	})

	It("should keep VMBDA VirtualImage volume when the image is not resolved yet", func() {
		volName := GenerateVIDiskName("vi-hotplug")
		kvvm := newKVVMWithVMBDAImageVolume(volName)

		err := syncAttachedVMBDAHotplugVolumes(
			kvvm,
			nil,
			map[string]*v1alpha2.VirtualImage{},
			nil,
			map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment{
				{Kind: v1alpha2.VMBDAObjectRefKindVirtualImage, Name: "vi-hotplug"}: nil,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].Name).To(Equal(volName))
	})
})

func newKVVMWithVMBDAImageVolume(volName string) *KVVM {
	kvvm := NewEmptyKVVM(
		namespacedName("test-vm", "test-ns"),
		KVVMOptions{},
	)
	kvvm.Resource.Spec.Template.Spec.Volumes = []virtv1.Volume{
		{
			Name: volName,
			VolumeSource: virtv1.VolumeSource{
				ContainerDisk: &virtv1.ContainerDiskSource{
					Image:        "dvcr.example/image:tag",
					Hotpluggable: true,
				},
			},
		},
	}
	kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks = []virtv1.Disk{
		{Name: volName},
	}
	return kvvm
}

var _ = Describe("ApplyMigrationVolumes", func() {
	const (
		vmName      = "test-vm"
		vmNamespace = "test-ns"
		diskName    = "data-disk"
		sourcePVC   = "pvc-source"
		targetPVC   = "pvc-target"
	)

	It("should switch hotplugged VMBDA disk to migration target PVC", func() {
		kvvm := newKVVMWithVMBDAVolume(sourcePVC)
		vm := &v1alpha2.VirtualMachine{
			Status: v1alpha2.VirtualMachineStatus{
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{
						Kind:       v1alpha2.DiskDevice,
						Name:       diskName,
						Hotplugged: true,
					},
				},
			},
		}
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:       diskName,
				Namespace:  vmNamespace,
				UID:        "vd-uid",
				Generation: 1,
			},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: sourcePVC},
				Conditions: []metav1.Condition{{
					Type:               vdcondition.MigratingType.String(),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
					Reason:             "Migrating",
				}},
				MigrationState: v1alpha2.VirtualDiskMigrationState{
					SourcePVC:      sourcePVC,
					TargetPVC:      targetPVC,
					StartTimestamp: metav1.Now(),
				},
			},
		}

		err := ApplyMigrationVolumes(kvvm, vm, map[string]*v1alpha2.VirtualDisk{diskName: vd})
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim).NotTo(BeNil())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(targetPVC))
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.Hotpluggable).To(BeTrue())
	})
})

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

			cleanupRemovedStaticDisks(kvvm, specDiskNames, hotpluggableVolumes, nil, false)

			// Should remove old-disk-1 (hotpluggable) and old-disk-2 (non-hotpluggable)
			// because VM is stopped
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(0))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(0))
		})

		It("should not remove disk attached via VMBDA when VM is stopped", func() {
			specDiskNames := map[string]struct{}{
				newDisk1Name: {},
			}
			hotpluggableVolumes := map[string]struct{}{}

			// Simulate disk attached via VMBDA
			vmbdaDiskNames := map[string]struct{}{
				oldDisk1Name: {},
			}

			cleanupRemovedStaticDisks(kvvm, specDiskNames, hotpluggableVolumes, vmbdaDiskNames, false)

			// old-disk-1 should stay because it's attached via VMBDA
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].Name).To(Equal(oldDisk1Name))
			// old-disk-2 should be removed because it's not in spec and not attached via VMBDA
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks[0].Name).To(Equal(oldDisk1Name))
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

			cleanupRemovedStaticDisks(kvvm, specDiskNames, hotpluggableVolumes, nil, true)

			// Should only remove old-disk-2 (non-hotpluggable)
			// old-disk-1 should stay because it's hotpluggable
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].Name).To(Equal(oldDisk1Name))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks[0].Name).To(Equal(oldDisk1Name))
		})
	})
})

var _ = Describe("setBlockDeviceDisk", func() {
	const (
		viName  = "vi-image"
		vdName  = "vd-data"
		viPVC   = "vi-pvc"
		vdPVC   = "vd-pvc"
		viImage = "dvcr.example/vi:tag"
	)

	newVI := func(storage v1alpha2.StorageType, format string) *v1alpha2.VirtualImage {
		return &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: viName, Namespace: "test-ns", UID: "vi-uid"},
			Spec:       v1alpha2.VirtualImageSpec{Storage: storage},
			Status: v1alpha2.VirtualImageStatus{
				Format: format,
				Target: v1alpha2.VirtualImageStatusTarget{
					PersistentVolumeClaim: viPVC,
					RegistryURL:           viImage,
				},
			},
		}
	}

	setDisk := func(bd v1alpha2.BlockDeviceSpecRef, vi *v1alpha2.VirtualImage, vd *v1alpha2.VirtualDisk) *KVVM {
		kvvm := NewEmptyKVVM(namespacedName("vm", "vm-ns"), KVVMOptions{EnableParavirtualization: true})
		err := setBlockDeviceDisk(
			kvvm, bd, 0, false,
			map[string]*v1alpha2.VirtualDisk{vdName: vd},
			map[string]*v1alpha2.VirtualImage{viName: vi},
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
		return kvvm
	}

	It("attaches a PVC-backed VirtualImage as a read-only disk", func() {
		vi := newVI(v1alpha2.StoragePersistentVolumeClaim, "qcow2")
		kvvm := setDisk(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ImageDevice, Name: viName}, vi, nil)

		disk := kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks[0]
		Expect(disk.Disk).NotTo(BeNil())
		Expect(disk.Disk.ReadOnly).To(BeTrue())

		pvc := kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim
		Expect(pvc).NotTo(BeNil())
		Expect(pvc.ClaimName).To(Equal(viPVC))
		Expect(pvc.ReadOnly).To(BeTrue())
	})

	It("attaches an ISO PVC-backed VirtualImage as a cdrom with a read-only PVC", func() {
		vi := newVI(v1alpha2.StoragePersistentVolumeClaim, "iso")
		kvvm := setDisk(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ImageDevice, Name: viName}, vi, nil)

		disk := kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks[0]
		Expect(disk.CDRom).NotTo(BeNil())

		pvc := kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim
		Expect(pvc).NotTo(BeNil())
		Expect(pvc.ReadOnly).To(BeTrue())
	})

	It("attaches a registry-backed VirtualImage as a container disk", func() {
		vi := newVI(v1alpha2.StorageContainerRegistry, "qcow2")
		kvvm := setDisk(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ImageDevice, Name: viName}, vi, nil)

		disk := kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks[0]
		Expect(disk.Disk).NotTo(BeNil())

		cd := kvvm.Resource.Spec.Template.Spec.Volumes[0].ContainerDisk
		Expect(cd).NotTo(BeNil())
		Expect(cd.Image).To(Equal(viImage))
	})

	It("attaches a VirtualDisk as a writable disk", func() {
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: vdName, Namespace: "test-ns", UID: "vd-uid"},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: vdPVC},
			},
		}
		kvvm := setDisk(v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: vdName}, nil, vd)

		disk := kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks[0]
		Expect(disk.Disk).NotTo(BeNil())
		Expect(disk.Disk.ReadOnly).To(BeFalse())

		pvc := kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim
		Expect(pvc).NotTo(BeNil())
		Expect(pvc.ClaimName).To(Equal(vdPVC))
		Expect(pvc.ReadOnly).To(BeFalse())
	})
})

func namespacedName(name, namespace string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: namespace}
}
