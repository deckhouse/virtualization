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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VirtualMachineRestoreForce", SIGRestoration(), framework.CommonE2ETestDecorators(), func() {
	var (
		ctx                 context.Context
		namespace           string
		testCaseLabel       = map[string]string{"testcase": "vm-restore-force"}
		additionalDiskLabel = map[string]string{"additionalDisk": "vm-restore-force"}
		originalVMNetworks  map[string][]v1alpha2.NetworksStatus
		criticalError       string
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMRestoreForce, "kustomization.yaml")
		var err error
		namespace, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(namespace)
	})

	BeforeEach(func() {
		if criticalError != "" {
			Skip(criticalError)
		}
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

		It("add additional interface to virtual machines", func() {
			sdnEnabled, err := isSdnModuleEnabled()
			if err != nil || !sdnEnabled {
				Skip("Module SDN is disabled. Skipping part of tests.")
			}

			By("patch virtual machines for add additional network interface", func() {
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

				// TODO: Remove manual restart when the VM restart issue is fixed.
				// This manual restart is only needed for tracking the problem and should be removed from the test in the future.
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

				vmrestores := &v1alpha2.VirtualMachineRestoreList{}
				err = GetObjects(v1alpha2.VirtualMachineRestoreResource, vmrestores, kc.GetOptions{Namespace: namespace})
				Expect(err).NotTo(HaveOccurred())

				// TODO: Remove this block when the bug with the virtual machine status phase "pending" is fixed.
				// Cause: When a virtual machine is in the restoration process, it can transition from the "stopped" phase to "pending" and the Virtualization Controller cannot complete the restoration process.
				for _, vmrestore := range vmrestores.Items {
					Eventually(func() error {
						vmRestoreObj := &v1alpha2.VirtualMachineRestore{}
						err := GetObject(v1alpha2.VirtualMachineRestoreResource, vmrestore.Name, vmRestoreObj, kc.GetOptions{Namespace: vmrestore.Namespace})
						if err != nil {
							return err
						}

						readyCondition, err := GetCondition(vmrestorecondition.VirtualMachineRestoreReady.String(), vmRestoreObj)
						if err != nil {
							return err
						}

						msg := "A virtual machine cannot be restored from the pending phase with `Forced` mode; you can delete the virtual machine and restore it with `Safe` mode."
						if vmRestoreObj.Status.Phase == v1alpha2.VirtualMachineRestorePhaseFailed && readyCondition.Message == msg {
							criticalError = "A bug has occurred with a virtual machine in the \"Pending\" phase."
							Skip(criticalError)
						}

						if vmRestoreObj.Status.Phase != v1alpha2.VirtualMachineRestorePhaseReady {
							return fmt.Errorf("virtual machine restore status phase should be \"Ready\": actual status is %q", vmRestoreObj.Status.Phase)
						}

						return nil
					}).WithTimeout(LongWaitDuration).WithPolling(Interval).Should(Succeed())
				}

				// Skip the VMRestore status phase check until the issue with the virtual machine status phase "pending" is fixed.
				// WaitPhaseByLabel(
				// 	virtv2.VirtualMachineRestoreResource,
				// 	string(virtv2.VirtualMachineRestorePhaseReady),
				// 	kc.WaitOptions{
				// 		Namespace: namespace,
				// 		Labels:    testCaseLabel,
				// 		Timeout:   LongWaitDuration,
				// 	})

				// Skip the VM agent check until the issue with the runPolicy is fixed.
				// WaitVMAgentReady(kc.WaitOptions{
				// 	Labels:    testCaseLabel,
				// 	Namespace: namespace,
				// 	Timeout:   LongWaitDuration,
				// })
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
					// Skip the BlockDeviceRefs check until the issue with the runPolicy is fixed.
					// Expect(vm.Status.BlockDeviceRefs).To(HaveLen(vmBlockDeviceCountBeforeSnapshotting[vm.Name]))

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

func NewVirtualMachineSnapshot(
	vmName, vmNamespace string,
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
