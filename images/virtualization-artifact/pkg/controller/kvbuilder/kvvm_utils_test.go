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
	"maps"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
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
			nil,
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
			nil,
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

	It("should keep VMBDA VirtualDisk volume when the disk is not resolved yet", func() {
		kvvm := newKVVMWithVMBDAVolume(sourcePVC)

		err := syncAttachedVMBDAHotplugVolumes(
			kvvm,
			nil,
			map[string]*v1alpha2.VirtualDisk{},
			nil,
			nil,
			map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment{
				{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: diskName}: nil,
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
	})

	It("should keep VMBDA ClusterVirtualImage volume when the image is not resolved yet", func() {
		volName := GenerateCVIDiskName("cvi-hotplug")
		kvvm := newKVVMWithVMBDAImageVolume(volName)

		err := syncAttachedVMBDAHotplugVolumes(
			kvvm,
			nil,
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

// A VMBDA-attached block device: the volume the running instance carries and the caches the
// builder rebuilds that volume from. The gates are common to every kind, the rebuild is where
// the builder branches per kind, so the kinds are tabled there.
type hotplugCase struct {
	ref       v1alpha2.VMBDAObjectRef
	volume    virtv1.Volume
	vdByName  map[string]*v1alpha2.VirtualDisk
	viByName  map[string]*v1alpha2.VirtualImage
	cviByName map[string]*v1alpha2.ClusterVirtualImage
}

const (
	hotplugNamespace = "test-ns"
	hotplugClaim     = "pvc-hotplug"
	hotplugImage     = "dvcr.example/image:tag"
)

func hotpluggedDisk() hotplugCase {
	return hotpluggedDiskNamed("data-disk")
}

func hotpluggedDiskNamed(name string) hotplugCase {
	vd := &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: hotplugNamespace, UID: types.UID("uid-" + name)},
		Status: v1alpha2.VirtualDiskStatus{
			Target: v1alpha2.DiskTarget{PersistentVolumeClaim: hotplugClaim + "-" + name},
		},
	}

	return hotplugCase{
		ref:      v1alpha2.VMBDAObjectRef{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: name},
		volume:   hotplugPVCVolume(GenerateVDDiskName(name), hotplugClaim+"-"+name, false),
		vdByName: map[string]*v1alpha2.VirtualDisk{name: vd},
	}
}

func hotpluggedImageOnPVC() hotplugCase {
	const name = "image-on-pvc"
	vi := &v1alpha2.VirtualImage{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: hotplugNamespace, UID: "vi-pvc-uid"},
		Spec:       v1alpha2.VirtualImageSpec{Storage: v1alpha2.StorageKubernetes},
		Status: v1alpha2.VirtualImageStatus{
			Target: v1alpha2.VirtualImageStatusTarget{PersistentVolumeClaim: hotplugClaim},
		},
	}

	return hotplugCase{
		ref: v1alpha2.VMBDAObjectRef{Kind: v1alpha2.VMBDAObjectRefKindVirtualImage, Name: name},
		// An image is immutable, so a PVC-backed one is mounted read-only.
		volume:   hotplugPVCVolume(GenerateVIDiskName(name), hotplugClaim, true),
		viByName: map[string]*v1alpha2.VirtualImage{name: vi},
	}
}

func hotpluggedImageInRegistry() hotplugCase {
	const name = "image-in-registry"
	vi := &v1alpha2.VirtualImage{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: hotplugNamespace, UID: "vi-registry-uid"},
		Spec:       v1alpha2.VirtualImageSpec{Storage: v1alpha2.StorageContainerRegistry},
		Status: v1alpha2.VirtualImageStatus{
			Target: v1alpha2.VirtualImageStatusTarget{RegistryURL: hotplugImage},
		},
	}

	return hotplugCase{
		ref:      v1alpha2.VMBDAObjectRef{Kind: v1alpha2.VMBDAObjectRefKindVirtualImage, Name: name},
		volume:   hotplugContainerDiskVolume(GenerateVIDiskName(name), hotplugImage),
		viByName: map[string]*v1alpha2.VirtualImage{name: vi},
	}
}

func hotpluggedClusterImage() hotplugCase {
	const name = "cluster-image"
	cvi := &v1alpha2.ClusterVirtualImage{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: "cvi-uid"},
		Status: v1alpha2.ClusterVirtualImageStatus{
			Target: v1alpha2.ClusterVirtualImageStatusTarget{RegistryURL: hotplugImage},
		},
	}

	return hotplugCase{
		ref:       v1alpha2.VMBDAObjectRef{Kind: v1alpha2.VMBDAObjectRefKindClusterVirtualImage, Name: name},
		volume:    hotplugContainerDiskVolume(GenerateCVIDiskName(name), hotplugImage),
		cviByName: map[string]*v1alpha2.ClusterVirtualImage{name: cvi},
	}
}

func hotplugPVCVolume(name, claimName string, readOnly bool) virtv1.Volume {
	return virtv1.Volume{
		Name: name,
		VolumeSource: virtv1.VolumeSource{
			PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
					ReadOnly:  readOnly,
				},
				Hotpluggable: true,
			},
		},
	}
}

func hotplugContainerDiskVolume(name, image string) virtv1.Volume {
	return virtv1.Volume{
		Name: name,
		VolumeSource: virtv1.VolumeSource{
			ContainerDisk: &virtv1.ContainerDiskSource{
				Image:        image,
				Hotpluggable: true,
			},
		},
	}
}

func runningInstance(volumes ...virtv1.Volume) *virtv1.VirtualMachineInstance {
	disks := make([]virtv1.Disk, 0, len(volumes))
	for _, volume := range volumes {
		disks = append(disks, virtv1.Disk{
			Name:        volume.Name,
			DiskDevice:  virtv1.DiskDevice{Disk: &virtv1.DiskTarget{Bus: virtv1.DiskBusSCSI}},
			ErrorPolicy: ptr.To(virtv1.DiskErrorPolicyReport),
		})
	}

	return &virtv1.VirtualMachineInstance{
		Spec: virtv1.VirtualMachineInstanceSpec{
			Volumes: volumes,
			Domain:  virtv1.DomainSpec{Devices: virtv1.Devices{Disks: disks}},
		},
		Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
	}
}

func deletingVMBDA() *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return &v1alpha2.VirtualMachineBlockDeviceAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "vmbda",
			Namespace:         hotplugNamespace,
			DeletionTimestamp: ptr.To(metav1.Now()),
			Finalizers:        []string{"test"},
		},
	}
}

func newHotplugKVVM(volumes ...virtv1.Volume) *KVVM {
	kvvm := NewEmptyKVVM(namespacedName("test-vm", hotplugNamespace), KVVMOptions{})
	kvvm.Resource.Spec.Template.Spec.Volumes = volumes
	for _, v := range volumes {
		kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks = append(
			kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks, virtv1.Disk{Name: v.Name})
	}
	return kvvm
}

func syncHotplugCases(
	kvvm *KVVM,
	kvvmi *virtv1.VirtualMachineInstance,
	vmbdas []*v1alpha2.VirtualMachineBlockDeviceAttachment,
	cases ...hotplugCase,
) error {
	vdByName := make(map[string]*v1alpha2.VirtualDisk)
	viByName := make(map[string]*v1alpha2.VirtualImage)
	cviByName := make(map[string]*v1alpha2.ClusterVirtualImage)
	vmbdaByRef := make(map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment)

	for _, c := range cases {
		maps.Copy(vdByName, c.vdByName)
		maps.Copy(viByName, c.viByName)
		maps.Copy(cviByName, c.cviByName)
		vmbdaByRef[c.ref] = vmbdas
	}

	return syncAttachedVMBDAHotplugVolumes(kvvm, kvvmi, vdByName, viByName, cviByName, vmbdaByRef)
}

var _ = Describe("syncAttachedVMBDAHotplugVolumes: hotplug volume dropped from the KVVM", func() {
	// The VirtualMachine and its instance must converge on their own: while the two disagree
	// on volumes, VolumesSynced stays false and the virtual machine cannot migrate at all.
	DescribeTable("should restore the volume the instance still runs",
		func(newCase func() hotplugCase) {
			hotplug := newCase()
			kvvm := newHotplugKVVM()
			kvvmi := runningInstance(hotplug.volume)

			Expect(syncHotplugCases(kvvm, kvvmi, nil, hotplug)).To(Succeed())

			// VolumesSynced compares both arrays with DeepEqual, so a volume restored with
			// different content, or in a different position, never converges.
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(Equal(kvvmi.Spec.Volumes))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
		},
		Entry("VirtualDisk", hotpluggedDisk),
		Entry("VirtualImage stored in a PVC", hotpluggedImageOnPVC),
		Entry("VirtualImage stored in the registry", hotpluggedImageInRegistry),
		Entry("ClusterVirtualImage", hotpluggedClusterImage),
	)

	DescribeTable("should leave a volume that is still in the KVVM as it is",
		func(newCase func() hotplugCase) {
			hotplug := newCase()
			kvvm := newHotplugKVVM(hotplug.volume)

			Expect(syncHotplugCases(kvvm, runningInstance(hotplug.volume), nil, hotplug)).To(Succeed())

			Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(Equal([]virtv1.Volume{hotplug.volume}))
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
		},
		Entry("VirtualDisk", hotpluggedDisk),
		Entry("VirtualImage stored in a PVC", hotpluggedImageOnPVC),
		Entry("VirtualImage stored in the registry", hotpluggedImageInRegistry),
		Entry("ClusterVirtualImage", hotpluggedClusterImage),
	)

	// The gates do not branch per block device kind, so one kind covers them all.
	DescribeTable("should not restore the volume",
		func(setup func(kvvm *KVVM, hotplug hotplugCase) (*virtv1.VirtualMachineInstance, []*v1alpha2.VirtualMachineBlockDeviceAttachment)) {
			hotplug := hotpluggedDisk()
			kvvm := newHotplugKVVM()
			kvvmi, vmbdas := setup(kvvm, hotplug)

			Expect(syncHotplugCases(kvvm, kvvmi, vmbdas, hotplug)).To(Succeed())

			Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(BeEmpty())
			Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(BeEmpty())
		},
		Entry("when the virtual machine is stopped",
			func(_ *KVVM, _ hotplugCase) (*virtv1.VirtualMachineInstance, []*v1alpha2.VirtualMachineBlockDeviceAttachment) {
				return nil, nil
			}),
		Entry("when the instance is no longer running",
			func(_ *KVVM, hotplug hotplugCase) (*virtv1.VirtualMachineInstance, []*v1alpha2.VirtualMachineBlockDeviceAttachment) {
				kvvmi := runningInstance(hotplug.volume)
				kvvmi.Status.Phase = virtv1.Succeeded
				return kvvmi, nil
			}),
		Entry("while the volume is being unplugged",
			func(kvvm *KVVM, hotplug hotplugCase) (*virtv1.VirtualMachineInstance, []*v1alpha2.VirtualMachineBlockDeviceAttachment) {
				kvvm.Resource.Status.VolumeRequests = []virtv1.VirtualMachineVolumeRequest{{
					RemoveVolumeOptions: &virtv1.RemoveVolumeOptions{Name: hotplug.volume.Name},
				}}
				return runningInstance(hotplug.volume), nil
			}),
		Entry("when every VMBDA of the block device is being deleted",
			func(_ *KVVM, hotplug hotplugCase) (*virtv1.VirtualMachineInstance, []*v1alpha2.VirtualMachineBlockDeviceAttachment) {
				return runningInstance(hotplug.volume), []*v1alpha2.VirtualMachineBlockDeviceAttachment{deletingVMBDA()}
			}),
	)

	It("should not restore a volume the instance carries without its disk", func() {
		hotplug := hotpluggedDisk()
		// A volume without its disk is rejected by the kubevirt webhook, and the whole
		// reconcile fails with it.
		kvvmi := runningInstance(hotplug.volume)
		kvvmi.Spec.Domain.Devices.Disks = nil
		kvvm := newHotplugKVVM()

		Expect(syncHotplugCases(kvvm, kvvmi, nil, hotplug)).To(Succeed())

		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(BeEmpty())
	})

	It("should not restore a volume the instance does not carry as hotpluggable", func() {
		hotplug := hotpluggedDisk()
		kvvmi := runningInstance(hotplug.volume)
		kvvmi.Spec.Volumes[0].PersistentVolumeClaim.Hotpluggable = false
		kvvm := newHotplugKVVM()

		Expect(syncHotplugCases(kvvm, kvvmi, nil, hotplug)).To(Succeed())

		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(BeEmpty())
	})

	It("should restore the volume while only one VMBDA of the disk is being deleted", func() {
		hotplug := hotpluggedDisk()
		vmbdas := []*v1alpha2.VirtualMachineBlockDeviceAttachment{
			deletingVMBDA(),
			{ObjectMeta: metav1.ObjectMeta{Name: "vmbda-live", Namespace: hotplugNamespace}},
		}
		kvvmi := runningInstance(hotplug.volume)
		kvvm := newHotplugKVVM()

		Expect(syncHotplugCases(kvvm, kvvmi, vmbdas, hotplug)).To(Succeed())

		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(Equal(kvvmi.Spec.Volumes))
	})

	It("should restore the volume at the position the instance keeps it", func() {
		first, second := hotpluggedDiskNamed("first-disk"), hotpluggedDiskNamed("second-disk")
		kvvmi := runningInstance(first.volume, second.volume)
		// Only the first volume fell out of the KVVM.
		kvvm := newHotplugKVVM(second.volume)

		Expect(syncHotplugCases(kvvm, kvvmi, nil, first, second)).To(Succeed())

		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(Equal(kvvmi.Spec.Volumes))
	})

	It("should restore the claim the instance actually runs", func() {
		hotplug := hotpluggedDisk()
		// The disk has been migrated: the instance runs the target claim while the
		// VirtualDisk status still points at the source one.
		kvvmi := runningInstance(hotplugPVCVolume(hotplug.volume.Name, "pvc-target", false))
		kvvm := newHotplugKVVM()

		Expect(syncHotplugCases(kvvm, kvvmi, nil, hotplug)).To(Succeed())

		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(Equal(kvvmi.Spec.Volumes))
	})

	It("should restore the volume when the block device is missing from the caches", func() {
		hotplug := hotpluggedDisk()
		// A volume that fell out of the KVVM also falls out of .status.blockDeviceRefs, and
		// that is what the block device caches are built from: the restore cannot rely on them.
		hotplug.vdByName = nil
		kvvmi := runningInstance(hotplug.volume)
		kvvm := newHotplugKVVM()

		Expect(syncHotplugCases(kvvm, kvvmi, nil, hotplug)).To(Succeed())

		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(Equal(kvvmi.Spec.Volumes))
	})
})

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

	DescribeTable("should pin the target PVC depending on the Migrating reason",
		func(reason, expectedPVC string) {
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
						Reason:             reason,
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
			Expect(kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(expectedPVC))
		},
		// The revert is waiting for the target claim: release it so the volumes can be reverted.
		Entry("waiting for the target claim to be released",
			vdcondition.MigratingWaitForTargetVolumeReleaseReason.String(), sourcePVC),
		// The migration has succeeded and the guest already runs on the target: keep it pinned.
		Entry("waiting for the source claim to be released",
			vdcondition.MigratingWaitForSourceVolumeReleaseReason.String(), targetPVC),
		Entry("migration in progress",
			vdcondition.MigratingInProgressReason.String(), targetPVC),
	)
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

// applyBlockDeviceRefs is the first gate of the two that decide a disk bus: it
// marks static disks of a stopped VM hotpluggable so they can be detached later
// without a restart, and a hotpluggable disk is then pinned to virtio-scsi by
// SetDisk regardless of the osType preset. That is how a Windows XP boot disk
// silently ended up on a controller it has no driver for: the Legacy osType asked
// for ide, but enableParavirtualization defaults to true, so the disk was marked
// hotpluggable and the bus followed the mark instead of the preset. The device
// matrix in kvvm_devices_test.go covers the second gate — it is handed the answer
// this function produces.
var _ = Describe("applyBlockDeviceRefs", func() {
	const (
		vdName = "data"
		vdPVC  = "vd-pvc"
	)

	newVM := func(osType v1alpha2.OsType, paravirt bool) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "vm-ns"},
			Spec: v1alpha2.VirtualMachineSpec{
				OsType:                   osType,
				EnableParavirtualization: ptr.To(paravirt),
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{Kind: v1alpha2.DiskDevice, Name: vdName},
				},
			},
		}
	}

	apply := func(vm *v1alpha2.VirtualMachine, isVmRunning bool) *KVVM {
		kvvm := NewEmptyKVVM(namespacedName("vm", "vm-ns"), KVVMOptions{
			OsType:                   vm.Spec.OsType,
			EnableParavirtualization: vm.Spec.IsParavirtualizationEnabled(),
		})
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: vdName, Namespace: "vm-ns", UID: "vd-uid"},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: vdPVC},
			},
		}
		var kvvmi *virtv1.VirtualMachineInstance
		if isVmRunning {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
			}
		}
		Expect(applyBlockDeviceRefs(
			kvvm, vm, kvvmi,
			map[string]*v1alpha2.VirtualDisk{vdName: vd}, nil, nil, nil,
		)).To(Succeed())
		return kvvm
	}

	hotpluggableOf := func(kvvm *KVVM) bool {
		Expect(kvvm.Resource.Spec.Template.Spec.Volumes).To(HaveLen(1))
		pvc := kvvm.Resource.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim
		Expect(pvc).NotTo(BeNil())
		return pvc.Hotpluggable
	}

	busOf := func(kvvm *KVVM) virtv1.DiskBus {
		Expect(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks).To(HaveLen(1))
		disk := kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks[0].Disk
		Expect(disk).NotTo(BeNil())
		return disk.Bus
	}

	DescribeTable("marks a static disk of a stopped VM hotpluggable",
		func(osType v1alpha2.OsType, paravirt, wantHotpluggable bool, wantBus virtv1.DiskBus) {
			kvvm := apply(newVM(osType, paravirt), false)
			Expect(hotpluggableOf(kvvm)).To(Equal(wantHotpluggable))
			Expect(busOf(kvvm)).To(Equal(wantBus))
		},
		Entry("Generic, paravirtualized", v1alpha2.GenericOs, true, true, virtv1.DiskBusSCSI),
		Entry("Generic, emulated", v1alpha2.GenericOs, false, false, virtv1.DiskBusSATA),
		Entry("Windows, paravirtualized", v1alpha2.Windows, true, true, virtv1.DiskBusSCSI),
		Entry("Windows, emulated", v1alpha2.Windows, false, false, virtv1.DiskBusSATA),
		// Legacy opts out of the semi-dynamic mode in both modes: the mark would take
		// the boot disk off the bus the osType deliberately picked — virtio-blk with
		// paravirtualization on, ide with it off — and put it on virtio-scsi, for which
		// no driver exists for these guests.
		Entry("Legacy, paravirtualized", v1alpha2.LegacyOs, true, false, virtv1.DiskBusVirtio),
		Entry("Legacy, emulated", v1alpha2.LegacyOs, false, false, virtv1.DiskBusIDE),
	)

	It("does not mark a static disk of a running VM", func() {
		// A running VM keeps the disks it booted with: marking them now would change
		// the bus under a live guest.
		kvvm := apply(newVM(v1alpha2.GenericOs, true), true)
		Expect(hotpluggableOf(kvvm)).To(BeFalse())
		Expect(busOf(kvvm)).To(Equal(virtv1.DiskBusSCSI))
	})
})

var _ = Describe("setExtraPVCsAnnotation", func() {
	newKVVM := func(volumes ...virtv1.Volume) *KVVM {
		kvvm := NewEmptyKVVM(namespacedName("vm", "vm-ns"), KVVMOptions{})
		kvvm.Resource.Spec.Template.Spec.Volumes = volumes
		return kvvm
	}

	pvcVolume := func(name, claimName string, hotpluggable bool) virtv1.Volume {
		return virtv1.Volume{
			Name: name,
			VolumeSource: virtv1.VolumeSource{
				PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: claimName,
					},
					Hotpluggable: hotpluggable,
				},
			},
		}
	}

	annotationOf := func(kvvm *KVVM) (string, bool) {
		value, ok := kvvm.Resource.Spec.Template.ObjectMeta.GetAnnotations()[annotations.AnnSchedulerExtraPVCs]
		return value, ok
	}

	It("lists hotpluggable PVC volumes sorted, skipping static and non-PVC ones", func() {
		kvvm := newKVVM(
			pvcVolume("vd-b", "pvc-b", true),
			pvcVolume("vd-root", "pvc-root", false),
			pvcVolume("vd-a", "pvc-a", true),
			virtv1.Volume{
				Name: "cvi-image",
				VolumeSource: virtv1.VolumeSource{
					ContainerDisk: &virtv1.ContainerDiskSource{Image: "img", Hotpluggable: true},
				},
			},
		)

		setExtraPVCsAnnotation(kvvm)

		value, ok := annotationOf(kvvm)
		Expect(ok).To(BeTrue())
		Expect(value).To(Equal("pvc-a,pvc-b"))
	})

	It("removes a stale annotation when no hotpluggable PVC volumes are left", func() {
		kvvm := newKVVM(pvcVolume("vd-root", "pvc-root", false))
		kvvm.SetKVVMIAnnotation(annotations.AnnSchedulerExtraPVCs, "pvc-gone")

		setExtraPVCsAnnotation(kvvm)

		_, ok := annotationOf(kvvm)
		Expect(ok).To(BeFalse())
	})

	It("does not set the annotation on a VM without hotpluggable volumes", func() {
		kvvm := newKVVM(pvcVolume("vd-root", "pvc-root", false))

		setExtraPVCsAnnotation(kvvm)

		_, ok := annotationOf(kvvm)
		Expect(ok).To(BeFalse())
	})

	It("deduplicates volumes referencing the same claim", func() {
		kvvm := newKVVM(
			pvcVolume("vd-a", "pvc-shared", true),
			pvcVolume("vd-b", "pvc-shared", true),
		)

		setExtraPVCsAnnotation(kvvm)

		value, ok := annotationOf(kvvm)
		Expect(ok).To(BeTrue())
		Expect(value).To(Equal("pvc-shared"))
	})
})

func namespacedName(name, namespace string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: namespace}
}
