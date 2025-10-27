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

package legacy

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

var _ = Describe("VirtualMachineRestoreSafe", framework.CommonE2ETestDecorators(), func() {
	var (
		ctx                 context.Context
		namespace           string
		testCaseLabel       = map[string]string{"testcase": "vm-restore-safe"}
		additionalDiskLabel = map[string]string{"additionalDisk": "vm-restore-safe"}
		originalVMNetworks  map[string][]v1alpha2.NetworksStatus
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMRestoreSafe, "kustomization.yaml")
		var err error
		namespace, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(namespace)
	})

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, namespace)
		}
	})

	Context("When the virtualization resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMRestoreSafe},
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

		It("add additional interface to virtual machines", func() {
			sdnEnabled, err := isSdnModuleEnabled()
			if err != nil || !sdnEnabled {
				Skip("Module SDN is disabled. Skipping part of tests.")
			}
			By("patch virtual machine for add additional network interface", func() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vmNames := strings.Split(res.StdOut(), " ")
				Expect(vmNames).NotTo(BeEmpty())

				cmd := fmt.Sprintf("patch %s --namespace %s %s --type merge --patch '{\"spec\":{\"networks\":[{\"type\":\"Main\"},{\"type\":\"ClusterNetwork\",\"name\":\"cn-1003-for-e2e-test\"}]}}'", kc.ResourceVM, namespace, res.StdOut())
				patchRes := kubectl.RawCommand(cmd, ShortWaitDuration)
				Expect(patchRes.Error()).NotTo(HaveOccurred(), patchRes.StdErr())

				RebootVirtualMachinesByVMOP(testCaseLabel, namespace, vmNames...)
			})
			By("`VirtualMachine` agent should be ready after patching", func() {
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			By("remembering the .status.networks of each VM after patching", func() {
				vms := &v1alpha2.VirtualMachineList{}
				err := GetObjects(v1alpha2.VirtualMachineResource, vms, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				originalVMNetworks = make(map[string][]v1alpha2.NetworksStatus, len(vms.Items))
				for _, vm := range vms.Items {
					originalVMNetworks[vm.Name] = vm.Status.Networks
				}
			})
		})
	})

	Context("When the resources are ready to use", func() {
		It("restore the `VirtualMachines` with `Safe` mode", func() {
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

			By("Deleting `VirtualMachines` and their resources for `Safe` restoring", func() {
				result := kubectl.Delete(kc.DeleteOptions{
					Labels:    testCaseLabel,
					Namespace: namespace,
					Resource:  kc.ResourceVM,
				})
				Expect(result.Error()).NotTo(HaveOccurred(), result.GetCmd())

				result = kubectl.Delete(kc.DeleteOptions{
					AllFlag:        true,
					IgnoreNotFound: true,
					Namespace:      namespace,
					Resource:       v1alpha2.VirtualMachineIPAddressResource,
				})
				Expect(result.Error()).NotTo(HaveOccurred(), result.GetCmd())

				result = kubectl.Delete(kc.DeleteOptions{
					ExcludedLabels: []string{"additionalDisk"},
					Namespace:      namespace,
					Resource:       v1alpha2.VirtualDiskResource,
				})
				Expect(result.Error()).NotTo(HaveOccurred(), result.GetCmd())

				result = kubectl.Delete(kc.DeleteOptions{
					Labels:    testCaseLabel,
					Namespace: namespace,
					Resource:  v1alpha2.VirtualMachineBlockDeviceAttachmentResource,
				})
				Expect(result.Error()).NotTo(HaveOccurred(), result.GetCmd())

				vmipls, err := GetVMIPLByNamespace(namespace)
				Expect(err).NotTo(HaveOccurred())
				WaitResourcesByPhase(
					vmipls, v1alpha2.VirtualMachineIPAddressLeaseResource,
					string(v1alpha2.VirtualMachineIPAddressLeasePhaseReleased),
					kc.WaitOptions{Timeout: ShortTimeout},
				)

				Eventually(func() error {
					err := CheckResourceCount(v1alpha2.VirtualMachineResource, namespace, testCaseLabel, 0)
					if err != nil {
						return err
					}

					err = CheckResourceCount(v1alpha2.VirtualDiskResource, namespace, testCaseLabel, 0)
					if err != nil {
						return err
					}

					err = CheckResourceCount(v1alpha2.VirtualMachineIPAddressResource, namespace, map[string]string{}, 0)
					if err != nil {
						return err
					}

					err = CheckResourceCount(v1alpha2.VirtualMachineBlockDeviceAttachmentResource, namespace, testCaseLabel, 0)
					if err != nil {
						return err
					}

					return nil
				}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
			})

			By("Creating `VirtualMachineRestores`", func() {
				vmsnapshots := &v1alpha2.VirtualMachineSnapshotList{}
				err := GetObjects(v1alpha2.VirtualMachineSnapshotResource, vmsnapshots, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
				Expect(err).NotTo(HaveOccurred())

				for _, vmsnapshot := range vmsnapshots.Items {
					vmrestore := NewVirtualMachineRestore(&vmsnapshot, v1alpha2.RestoreModeSafe)
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
				// const (
				// 	testLabelKey        = "test-label"
				// 	testLabelValue      = "test-label-value"
				// 	testAnnotationKey   = "test-annotation"
				// 	testAnnotationValue = "test-annotation-value"
				// )

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

							// Skip the annotation and label checks until the issue with virtual disk restoration is fixed.
							// Cause: Sometimes, a virtual disk does not have annotations and labels from a virtual disk snapshot, causing the test to fail.
							// Expect(vd.Annotations).To(HaveKeyWithValue(testAnnotationKey, testAnnotationValue))
							// Expect(vd.Labels).To(HaveKeyWithValue(testLabelKey, testLabelValue))
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

		It("check the .status.networks of each VM after restore", func() {
			sdnEnabled, err := isSdnModuleEnabled()
			if err != nil || !sdnEnabled {
				Skip("Module SDN is disabled. Skipping part of tests.")
			}

			vmrestores := &v1alpha2.VirtualMachineRestoreList{}
			err = GetObjects(v1alpha2.VirtualMachineRestoreKind, vmrestores, kc.GetOptions{Namespace: namespace, Labels: testCaseLabel})
			Expect(err).NotTo(HaveOccurred())

			for _, restore := range vmrestores.Items {
				vmsnapshot := &v1alpha2.VirtualMachineSnapshot{}
				err := GetObject(v1alpha2.VirtualMachineSnapshotKind, restore.Spec.VirtualMachineSnapshotName, vmsnapshot, kc.GetOptions{Namespace: restore.Namespace})
				Expect(err).NotTo(HaveOccurred())

				vm := &v1alpha2.VirtualMachine{}
				err = GetObject(v1alpha2.VirtualMachineKind, vmsnapshot.Spec.VirtualMachineName, vm, kc.GetOptions{Namespace: vmsnapshot.Namespace})
				Expect(err).NotTo(HaveOccurred())
				// Skip the network checks until the issue with the virtual machine's MAC address is fixed.
				// Cause: Sometimes, a virtual machine has a different MAC address after restoration, causing the test to fail.
				// Expect(originalVMNetworks).To(HaveKeyWithValue(vm.Name, vm.Status.Networks))
			}
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

func CheckResourceCount(resource, namespace string, labels map[string]string, count int) error {
	result := kubectl.List(kc.Resource(resource), kc.GetOptions{
		IgnoreNotFound: true,
		Labels:         labels,
		Namespace:      namespace,
		Output:         "jsonpath='{.items[*].metadata.name}'",
	})
	if result.Error() != nil {
		return fmt.Errorf("failed to list %q: %s", resource, result.StdErr())
	}

	if result.StdOut() == "" {
		return nil
	}

	if len(strings.Split(result.StdOut(), " ")) != count {
		return fmt.Errorf("there should be %d %q", count, resource)
	}

	return nil
}

func GetVMIPLByNamespace(namespace string) ([]string, error) {
	vmipls := &v1alpha2.VirtualMachineIPAddressLeaseList{}
	err := GetObjects(v1alpha2.VirtualMachineIPAddressLeaseResource, vmipls, kc.GetOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(vmipls.Items))

	for _, vmipl := range vmipls.Items {
		if vmipl.Namespace == namespace {
			result = append(result, vmipl.Name)
		}
	}

	return result, nil
}
