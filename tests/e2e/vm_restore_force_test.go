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

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
				err := CheckReusableResources(ReusableResources{
					v1alpha2.VirtualMachineResource: &Counter{
						Expected: vmCount,
					},
					v1alpha2.VirtualDiskResource: &Counter{
						Expected: vdCount,
					},
					v1alpha2.VirtualImageResource: &Counter{
						Expected: viCount,
					},
					v1alpha2.VirtualMachineBlockDeviceAttachmentResource: &Counter{
						Expected: vmbdaCount,
					},
				}, kc.GetOptions{
					Namespace:      namespace,
					IgnoreNotFound: true,
				})
				if err == nil {
					return
				}
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
			By("`VirtualMachineBlockDeviceAttachment` should be attached", func() {
				WaitPhaseByLabel(
					v1alpha2.VirtualMachineBlockDeviceAttachmentKind,
					string(v1alpha2.BlockDeviceAttachmentPhaseAttached),
					kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: namespace,
						Timeout:   LongWaitDuration,
					})
			})
		})
	})

	Context("When the resources are ready to use", func() {
		It("restore the `VirtualMachines` with `forced` mode", func() {
			vms := &v1alpha2.VirtualMachineList{}
			vmBlockDeviceCountBeforeSnapshotting := make(map[string]int, len(vms.Items))

			By("Getting `VirtualMachines`", func() {
				err := GetObjects(v1alpha2.VirtualMachineResource, vms, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
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
						true,
						v1alpha2.KeepIPAddressAlways,
						testCaseLabel,
					)
					CreateResource(ctx, vmsnapshot)
				}
				WaitPhaseByLabel(
					v1alpha2.VirtualMachineSnapshotResource,
					string(v1alpha2.VirtualMachineSnapshotPhaseReady),
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
					newVmbda := NewVirtualMachineBlockDeviceAttachment(vm.Name, vm.Namespace, newDisk.Name, v1alpha2.VMBDAObjectRefKindVirtualDisk, additionalDiskLabel)
					CreateResource(ctx, newVmbda)

					WaitPhaseByLabel(
						v1alpha2.VirtualMachineBlockDeviceAttachmentResource,
						string(v1alpha2.BlockDeviceAttachmentPhaseAttached),
						kc.WaitOptions{
							Namespace: vm.Namespace,
							Labels:    additionalDiskLabel,
							Timeout:   LongWaitDuration,
						})
					err := GetObject(v1alpha2.VirtualMachineKind, vm.Name, &vm, kc.GetOptions{Namespace: vm.Namespace})
					Expect(err).NotTo(HaveOccurred())
					Expect(vm.Status.BlockDeviceRefs).To(HaveLen(vmBlockDeviceCountBeforeSnapshotting[vm.Name] + 1))
				}
			})

			By("Creating `VirtualMachineRestores`", func() {
				vmsnapshots := &v1alpha2.VirtualMachineSnapshotList{}
				err := GetObjects(v1alpha2.VirtualMachineSnapshotResource, vmsnapshots, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				for _, vmsnapshot := range vmsnapshots.Items {
					vmrestore := NewVirtualMachineRestore(&vmsnapshot, v1alpha2.RestoreModeForced)
					CreateResource(ctx, vmrestore)
				}
				WaitPhaseByLabel(
					v1alpha2.VirtualMachineRestoreResource,
					string(v1alpha2.VirtualMachineRestorePhaseReady),
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
				vmrestores := &v1alpha2.VirtualMachineRestoreList{}
				err := GetObjects(v1alpha2.VirtualMachineRestoreKind, vmrestores, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				for _, restore := range vmrestores.Items {
					vmsnapshot := &v1alpha2.VirtualMachineSnapshot{}
					err := GetObject(v1alpha2.VirtualMachineSnapshotKind, restore.Spec.VirtualMachineSnapshotName, vmsnapshot, kc.GetOptions{Namespace: restore.Namespace})
					Expect(err).NotTo(HaveOccurred())

					vm := &v1alpha2.VirtualMachine{}
					err = GetObject(v1alpha2.VirtualMachineKind, vmsnapshot.Spec.VirtualMachineName, vm, kc.GetOptions{Namespace: vmsnapshot.Namespace})
					Expect(err).NotTo(HaveOccurred())

					Expect(vm.Annotations).To(HaveKeyWithValue(annotations.AnnVMRestore, string(restore.UID)))
					Expect(vm.Status.BlockDeviceRefs).To(HaveLen(vmBlockDeviceCountBeforeSnapshotting[vm.Name]))

					for _, bd := range vm.Status.BlockDeviceRefs {
						if bd.Kind == v1alpha2.DiskDevice {
							vd := &v1alpha2.VirtualDisk{}
							err := GetObject(v1alpha2.VirtualDiskKind, bd.Name, vd, kc.GetOptions{Namespace: vm.Namespace})
							Expect(err).NotTo(HaveOccurred())
							Expect(vd.Annotations).To(HaveKeyWithValue(annotations.AnnVMRestore, string(restore.UID)))
						}

						if bd.VirtualMachineBlockDeviceAttachmentName != "" {
							vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{}
							err := GetObject(v1alpha2.VirtualMachineBlockDeviceAttachmentKind, bd.VirtualMachineBlockDeviceAttachmentName, vmbda, kc.GetOptions{Namespace: vm.Namespace})
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
						Resource: v1alpha2.VirtualMachineSnapshotResource,
						Labels:   testCaseLabel,
					},
					{
						Resource: v1alpha2.VirtualMachineRestoreResource,
						Labels:   testCaseLabel,
					},
					{
						Resource: v1alpha2.VirtualDiskResource,
						Labels:   additionalDiskLabel,
					},
					{
						Resource: v1alpha2.VirtualMachineBlockDeviceAttachmentResource,
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
	vmName, vmNamespace, storageClass string,
	requiredConsistency bool,
	keepIPaddress v1alpha2.KeepIPAddress,
	labels map[string]string,
) *v1alpha2.VirtualMachineSnapshot {
	return &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: vmNamespace,
			Labels:    labels,
		},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{
			VirtualMachineName:  vmName,
			RequiredConsistency: requiredConsistency,
			KeepIPAddress:       keepIPaddress,
		},
	}
}

func NewVirtualMachineRestore(vmsnapshot *v1alpha2.VirtualMachineSnapshot, restoreMode v1alpha2.RestoreMode) *v1alpha2.VirtualMachineRestore {
	return &v1alpha2.VirtualMachineRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmsnapshot.Spec.VirtualMachineName,
			Namespace: vmsnapshot.Namespace,
			Labels:    vmsnapshot.Labels,
		},
		Spec: v1alpha2.VirtualMachineRestoreSpec{
			RestoreMode:                restoreMode,
			VirtualMachineSnapshotName: vmsnapshot.Name,
		},
	}
}

func NewVirtualMachineBlockDeviceAttachment(vmName, vmNamespace, bdName string, bdKind v1alpha2.VMBDAObjectRefKind, labels map[string]string) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return &v1alpha2.VirtualMachineBlockDeviceAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bdName,
			Namespace: vmNamespace,
			Labels:    labels,
		},
		Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
			VirtualMachineName: vmName,
			BlockDeviceRef: v1alpha2.VMBDAObjectRef{
				Kind: bdKind,
				Name: bdName,
			},
		},
	}
}

func NewVirtualDisk(vdName, vdNamespace string, labels map[string]string, size *resource.Quantity) *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vdName,
			Namespace: vdNamespace,
			Labels:    labels,
		},
		Spec: v1alpha2.VirtualDiskSpec{
			PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
				Size: size,
			},
		},
	}
}
