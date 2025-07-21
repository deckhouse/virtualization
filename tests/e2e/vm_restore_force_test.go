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

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VirtualMachineRestoreForce", SIGRestoration(), ginkgoutil.CommonE2ETestDecorators(), func() {
	const (
		viCount    = 2
		vmCount    = 1
		vdCount    = 2
		vmbdaCount = 2
	)

	var (
		ctx                 context.Context
		cancel              context.CancelFunc
		storageClass        *storagev1.StorageClass
		volumeSnapshotClass string
		namespace           string
		testCaseLabel       = map[string]string{"testcase": "vm-restore-force"}
		additionalDiskLabel = map[string]string{"additionalDisk": "vm-restore-force"}
	)
	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}

		cancel()
	})

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMRestoreForce, "kustomization.yaml")
		var err error
		namespace, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		storageClass, err = GetDefaultStorageClass()
		Expect(err).NotTo(HaveOccurred(), "failed to get the `DefaultStorageClass`")
		volumeSnapshotClass, err = GetVolumeSnapshotClassName(storageClass)
		Expect(err).NotTo(HaveOccurred(), "failed to get the `VolumeSnapshotClass`")

		res := kubectl.Delete(kc.DeleteOptions{
			IgnoreNotFound: true,
			Labels:         testCaseLabel,
			Resource:       kc.ResourceCVI,
		})
		Expect(res.Error()).NotTo(HaveOccurred())
	})

	Context("When the virtualization resources are applied", func() {
		It("result should be succeeded", func() {
			if config.IsReusable() {
				CheckReusableResources(ReusableResources{
					virtv2.VirtualMachineResource: &Counter{
						Expected: vmCount,
					},
					virtv2.VirtualDiskResource: &Counter{
						Expected: vdCount,
					},
					virtv2.VirtualImageResource: &Counter{
						Expected: viCount,
					},
					virtv2.VirtualMachineBlockDeviceAttachmentResource: &Counter{
						Expected: vmbdaCount,
					},
				}, kc.GetOptions{
					Labels:         testCaseLabel,
					Namespace:      namespace,
					IgnoreNotFound: true,
				})
			}

			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMRestoreForce},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})

		It("checks the resources phase", func() {
			By("`VirtualMachine` agent should be ready", func() {
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the resources are ready to use", func() {
		It("restore the `VirtualMachines` with `forced` mode", func() {
			vms := &virtv2.VirtualMachineList{}
			vmBlockDeviceCountBeforeSnapshotting := make(map[string]int, len(vms.Items))

			By("Getting `VirtualMachines`", func() {
				err := GetObjects(virtv2.VirtualMachineResource, vms, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())
				for _, vm := range vms.Items {
					vmBlockDeviceCountBeforeSnapshotting[vm.Name] = len(vm.Status.BlockDeviceRefs)
				}
			})

			By("Creating `VirtualMachineSnapshots`", func() {
				for _, vm := range vms.Items {
					vmsnapshot := NewVirtualMachineSnapshot(
						vm.Name, vm.Namespace,
						storageClass.Name,
						volumeSnapshotClass,
						true,
						virtv2.KeepIPAddressAlways,
						testCaseLabel,
					)
					CreateResource(ctx, vmsnapshot)
				}
				WaitPhaseByLabel(
					virtv2.VirtualMachineSnapshotResource,
					string(virtv2.VirtualMachineSnapshotPhaseReady),
					kc.WaitOptions{
						Namespace: namespace,
						Labels:    testCaseLabel,
						Timeout:   LongWaitDuration,
					})
			})

			By("Attaching `VirtualDisk` after `VirtualMachine` snapshotting", func() {
				for i, vm := range vms.Items {
					vdName := fmt.Sprintf("%s-%d", "vd-attached-after-vm-snapshotting", i)
					newDisk := NewVirtualDisk(vdName, vm.Namespace, additionalDiskLabel, resource.NewQuantity(1*1024*1024, resource.BinarySI))
					CreateResource(ctx, newDisk)
					newVmbda := NewVirtualMachineBlockDeviceAttachment(vm.Name, vm.Namespace, newDisk.Name, virtv2.VMBDAObjectRefKindVirtualDisk, additionalDiskLabel)
					CreateResource(ctx, newVmbda)

					WaitPhaseByLabel(
						virtv2.VirtualMachineBlockDeviceAttachmentResource,
						string(virtv2.BlockDeviceAttachmentPhaseAttached),
						kc.WaitOptions{
							Namespace: vm.Namespace,
							Labels:    testCaseLabel,
							Timeout:   LongWaitDuration,
						})
					err := GetObject(virtv2.VirtualMachineKind, vm.Name, &vm, kc.GetOptions{Namespace: vm.Namespace})
					Expect(err).NotTo(HaveOccurred())
					Expect(vm.Status.BlockDeviceRefs).To(HaveLen(vmBlockDeviceCountBeforeSnapshotting[vm.Name] + 1))
				}
			})

			By("Creating `VirtualMachineRestores`", func() {
				vmsnapshots := &virtv2.VirtualMachineSnapshotList{}
				err := GetObjects(virtv2.VirtualMachineSnapshotResource, vmsnapshots, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				for _, vmsnapshot := range vmsnapshots.Items {
					vmrestore := NewVirtualMachineRestore(&vmsnapshot, virtv2.RestoreModeForced)
					CreateResource(ctx, vmrestore)
				}
				WaitPhaseByLabel(
					virtv2.VirtualMachineRestoreResource,
					string(virtv2.VirtualMachineRestorePhaseReady),
					kc.WaitOptions{
						Namespace: namespace,
						Labels:    testCaseLabel,
						Timeout:   LongWaitDuration,
					})

				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: namespace,
					Timeout:   LongWaitDuration,
				})
			})

			By("Checking the result of restoration", func() {
				vmrestores := &virtv2.VirtualMachineRestoreList{}
				err := GetObjects(virtv2.VirtualMachineRestoreKind, vmrestores, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				for _, restore := range vmrestores.Items {
					vmsnapshot := &virtv2.VirtualMachineSnapshot{}
					err := GetObject(virtv2.VirtualMachineSnapshotKind, restore.Spec.VirtualMachineSnapshotName, vmsnapshot, kc.GetOptions{Namespace: restore.Namespace})
					Expect(err).NotTo(HaveOccurred())

					vm := &virtv2.VirtualMachine{}
					err = GetObject(virtv2.VirtualMachineKind, vmsnapshot.Spec.VirtualMachineName, vm, kc.GetOptions{Namespace: vmsnapshot.Namespace})
					Expect(err).NotTo(HaveOccurred())

					Expect(vm.Annotations).To(HaveKeyWithValue(annotations.AnnVMRestore, string(restore.UID)))
					Expect(vm.Status.BlockDeviceRefs).To(HaveLen(vmBlockDeviceCountBeforeSnapshotting[vm.Name]))

					for _, bd := range vm.Status.BlockDeviceRefs {
						if bd.Kind == virtv2.DiskDevice {
							vd := &virtv2.VirtualDisk{}
							err := GetObject(virtv2.VirtualDiskKind, bd.Name, vd, kc.GetOptions{Namespace: vm.Namespace})
							Expect(err).NotTo(HaveOccurred())
							Expect(vd.Annotations).To(HaveKeyWithValue(annotations.AnnVMRestore, string(restore.UID)))
						}

						if bd.VirtualMachineBlockDeviceAttachmentName != "" {
							vmbda := &virtv2.VirtualMachineBlockDeviceAttachment{}
							err := GetObject(virtv2.VirtualMachineBlockDeviceAttachmentKind, bd.VirtualMachineBlockDeviceAttachmentName, vmbda, kc.GetOptions{Namespace: vm.Namespace})
							Expect(err).NotTo(HaveOccurred())
							Expect(vmbda.Annotations).To(HaveKeyWithValue(annotations.AnnVMRestore, string(restore.UID)))
						}
					}
				}
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			resourcesToDelete := ResourcesToDelete{
				AdditionalResources: []AdditionalResource{
					{
						Resource: virtv2.VirtualMachineSnapshotResource,
						Labels:   testCaseLabel,
					},
					{
						Resource: virtv2.VirtualMachineRestoreResource,
						Labels:   testCaseLabel,
					},
					{
						Resource: virtv2.VirtualDiskResource,
						Labels:   additionalDiskLabel,
					},
					{
						Resource: virtv2.VirtualMachineBlockDeviceAttachmentResource,
						Labels:   additionalDiskLabel,
					},
				},
			}

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.VMRestoreForce
			}

			DeleteTestCaseResources(namespace, resourcesToDelete)
		})
	})
})

func NewVirtualMachineSnapshot(
	vmName, vmNamespace, storageClass, volumeSnapshotClass string,
	requiredConsistency bool,
	keepIPaddress virtv2.KeepIPAddress,
	labels map[string]string,
) *virtv2.VirtualMachineSnapshot {
	return &virtv2.VirtualMachineSnapshot{
		ObjectMeta: v1.ObjectMeta{
			Name:      vmName,
			Namespace: vmNamespace,
			Labels:    labels,
		},
		Spec: virtv2.VirtualMachineSnapshotSpec{
			VirtualMachineName:  vmName,
			RequiredConsistency: requiredConsistency,
			KeepIPAddress:       keepIPaddress,
			VolumeSnapshotClasses: []virtv2.VolumeSnapshotClassName{
				{
					StorageClassName:        storageClass,
					VolumeSnapshotClassName: volumeSnapshotClass,
				},
			},
		},
	}
}

func NewVirtualMachineRestore(vmsnapshot *virtv2.VirtualMachineSnapshot, restoreMode virtv2.RestoreMode) *virtv2.VirtualMachineRestore {
	return &virtv2.VirtualMachineRestore{
		ObjectMeta: v1.ObjectMeta{
			Name:      vmsnapshot.Spec.VirtualMachineName,
			Namespace: vmsnapshot.Namespace,
			Labels:    vmsnapshot.Labels,
		},
		Spec: virtv2.VirtualMachineRestoreSpec{
			RestoreMode:                restoreMode,
			VirtualMachineSnapshotName: vmsnapshot.Name,
		},
	}
}

func NewVirtualMachineBlockDeviceAttachment(vmName, vmNamespace, bdName string, bdKind virtv2.VMBDAObjectRefKind, labels map[string]string) *virtv2.VirtualMachineBlockDeviceAttachment {
	return &virtv2.VirtualMachineBlockDeviceAttachment{
		ObjectMeta: v1.ObjectMeta{
			Name:      bdName,
			Namespace: vmNamespace,
			Labels:    labels,
		},
		Spec: virtv2.VirtualMachineBlockDeviceAttachmentSpec{
			VirtualMachineName: vmName,
			BlockDeviceRef: virtv2.VMBDAObjectRef{
				Kind: bdKind,
				Name: bdName,
			},
		},
	}
}

func NewVirtualDisk(vdName, vdNamespace string, labels map[string]string, size *resource.Quantity) *virtv2.VirtualDisk {
	return &virtv2.VirtualDisk{
		ObjectMeta: v1.ObjectMeta{
			Name:      vdName,
			Namespace: vdNamespace,
			Labels:    labels,
		},
		Spec: virtv2.VirtualDiskSpec{
			PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{
				Size: size,
			},
		},
	}
}
