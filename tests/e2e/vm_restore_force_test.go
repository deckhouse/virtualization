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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
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
	)
	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}

		cancel()
	})

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VmRestoreForce, "kustomization.yaml")
		var err error
		namespace, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
		conf.SetNamespace(namespace)

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
				Filename:       []string{conf.TestData.VmRestoreForce},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})

		It("checks the resources phase", func() {
			By("`VirtualMachine` agent should be ready", func() {
				WaitVmAgentReady(kc.WaitOptions{
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

			By("Getting `VirtualMachines`", func() {
				err := GetObjects(virtv2.VirtualMachineResource, vms, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())
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
			})

			By("Checking the result of restoration", func() {
				vms := &virtv2.VirtualMachineList{}
				err := GetObjects(virtv2.VirtualMachineKind, vms, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				for _, vm := range vms.Items {
					Expect(vm.Annotations).To(HaveKey(annotations.AnnVMRestore))
				}

				vds := &virtv2.VirtualDiskList{}
				err = GetObjects(virtv2.VirtualDiskKind, vds, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				for _, vd := range vds.Items {
					Expect(vd.Annotations).To(HaveKey(annotations.AnnVMRestore))
				}

				vmbdas := &virtv2.VirtualMachineBlockDeviceAttachmentList{}
				err = GetObjects(virtv2.VirtualMachineBlockDeviceAttachmentKind, vmbdas, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				for _, vmbda := range vmbdas.Items {
					Expect(vmbda.Annotations).To(HaveKey(annotations.AnnVMRestore))
				}

				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: namespace,
					Timeout:   LongWaitDuration,
				})
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
				},
			}

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.VmRestoreForce
			}

			DeleteTestCaseResources(resourcesToDelete)
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
