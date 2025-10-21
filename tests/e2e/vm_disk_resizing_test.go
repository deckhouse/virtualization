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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	cfg "github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("VirtualDiskResizing", framework.CommonE2ETestDecorators(), func() {
	const (
		vmCount   = 1
		diskCount = 3
	)
	var vmObj *v1alpha2.VirtualMachine
	var ns string
	testCaseLabel := map[string]string{"testcase": "disk-resizing"}

	BeforeAll(func() {
		// TODO: The test is being disabled because the new functionality for volume migration has introduced
		// an issue that results in the disappearance of the serial inside kvvmi.
		// This leads to errors during disk resizing. Remove Skip after fixing the issue.
		Skip("This test case is not working everytime. Should be fixed.")

		kustomization := fmt.Sprintf("%s/%s", conf.TestData.DiskResizing, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
	})

	Context("When the resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.DiskResizing},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When the virtual images are applied", func() {
		It("checks `VirtualImages` phase", func() {
			By(fmt.Sprintf("`VirtualImages` should be in the %q phases", v1alpha2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceVI, string(v1alpha2.ImageReady), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the virtual disks are applied", func() {
		It("checks `VirtualDisks` phase", func() {
			By(fmt.Sprintf("`VirtualDisks` should be in the %q phases", v1alpha2.DiskReady), func() {
				WaitPhaseByLabel(kc.ResourceVD, string(v1alpha2.DiskReady), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the virtual machine are applied", func() {
		It("checks `VirtualMachine` phase", func() {
			By("`VirtualMachine` agent should be ready", func() {
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		It("retrieves the test objects", func() {
			By("`VirtualMachine`", func() {
				vmObjs := &v1alpha2.VirtualMachineList{}
				err := GetObjects(v1alpha2.VirtualMachineResource, vmObjs, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to get `VirtualMachines`: %s", err)
				Expect(vmObjs.Items).To(HaveLen(vmCount), "there is only %d `VirtualMachine` in this test case", vmCount)
				vmObj = &vmObjs.Items[0]
				Expect(vmObj).ShouldNot(BeNil(), "failed to retrieve `VirtualMachine` object: %+v", vmObjs)
			})
		})
	})

	Context("When the virtual machine block device attachment is applied", func() {
		It("checks `VirtualMachineBlockDeviceAttachment` phase", func() {
			By(fmt.Sprintf("`VirtualMachineBlockDeviceAttachment` should be in the %q phases", v1alpha2.BlockDeviceAttachmentPhaseAttached), func() {
				WaitPhaseByLabel(kc.ResourceVMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Describe("Resizing", func() {
		Context("When the virtual machine is ready", func() {
			var (
				vmDisksBefore VirtualMachineDisks
				vmDisksAfter  VirtualMachineDisks
				err           error
			)

			It("obtains the disks metadata before resizing", func() {
				vmDisksBefore, err = GetVirtualMachineDisks(ns, vmObj.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmDisksBefore).Should(HaveLen(diskCount))
			})

			It("resizes the disks", func() {
				wg := &sync.WaitGroup{}
				res := kubectl.List(kc.ResourceVD, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				vds := strings.Split(res.StdOut(), " ")
				Expect(vds).Should(HaveLen(diskCount))
				addedSize := resource.NewQuantity(100*1024*1024, resource.BinarySI)
				wg.Add(2)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					By(fmt.Sprintf("`VirtualDisks` should be in the %q phase", v1alpha2.DiskResizing), func() {
						WaitPhaseByLabel(v1alpha2.VirtualDiskResource, string(v1alpha2.DiskResizing), kc.WaitOptions{
							Labels:    testCaseLabel,
							Namespace: ns,
							Timeout:   MaxWaitTimeout,
						})
					})
				}()
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					ResizeDisks(addedSize, conf, ns, vds...)
				}()
				wg.Wait()
			})

			It("checks `VirtualDisks`, `VirtualMachine` and `VirtualMachineBlockDeviceAttachment` phases", func() {
				By(fmt.Sprintf("`VirtualDisks` should be in the %q phases", v1alpha2.DiskReady), func() {
					WaitPhaseByLabel(kc.ResourceVD, string(v1alpha2.DiskReady), kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: ns,
						Timeout:   MaxWaitTimeout,
					})
				})
				By("`VirtualMachine` should be ready", func() {
					WaitVMAgentReady(kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: ns,
						Timeout:   MaxWaitTimeout,
					})
				})
				By(fmt.Sprintf("`VirtualMachineBlockDeviceAttachment` should be in the %q phases", v1alpha2.BlockDeviceAttachmentPhaseAttached), func() {
					WaitPhaseByLabel(kc.ResourceVMBDA, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: ns,
						Timeout:   MaxWaitTimeout,
					})
				})
				By("`BlockDevices` from the status should be attached", func() {
					WaitBlockDeviceRefsAttached(ns, vmObj.Name)
				})
			})

			It("obtains and compares the disks metadata after resizing", func() {
				Eventually(func() error {
					vmDisksAfter, err = GetVirtualMachineDisks(ns, vmObj.Name)
					if err != nil {
						return fmt.Errorf("failed to obtain disks metadata after resizing: %w", err)
					}
					if len(vmDisksAfter) != diskCount {
						return fmt.Errorf("quantity of the disk should be %d", diskCount)
					}
					for disk := range vmDisksBefore {
						sizeFromObjectBefore := vmDisksBefore[disk].SizeFromObject.Value()
						sizeFromObjectAfter := vmDisksAfter[disk].SizeFromObject.Value()
						if sizeFromObjectBefore >= sizeFromObjectAfter {
							return fmt.Errorf(
								"size from objects before must be lower than size after: before: %d, after: %d",
								sizeFromObjectBefore,
								sizeFromObjectAfter,
							)
						}
						sizeByLsblkBefore := vmDisksBefore[disk].SizeByLsblk.Value()
						sizeByLsblkAfter := vmDisksAfter[disk].SizeByLsblk.Value()
						if sizeByLsblkBefore >= sizeByLsblkAfter {
							return fmt.Errorf(
								"size by lsblk before must be lower than size after: before: %d, after: %d",
								sizeByLsblkBefore,
								sizeByLsblkAfter,
							)
						}
					}
					return nil
				}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ns, ResourcesToDelete{
				KustomizationDir: conf.TestData.DiskResizing,
			})
		})
	})
})

type VirtualMachineDisks map[string]DiskMetaData

type DiskMetaData struct {
	ID             string
	SizeByLsblk    *resource.Quantity
	SizeFromObject *resource.Quantity
}

const (
	DiskIDPrefix  = "scsi-0QEMU_QEMU_HARDDISK"
	CdRomIDPrefix = "scsi-0QEMU_QEMU_CD-ROM_drive-ua"
)

func WaitBlockDeviceRefsAttached(namespace string, vms ...string) {
	GinkgoHelper()
	Eventually(func() error {
		for _, vmName := range vms {
			vm := v1alpha2.VirtualMachine{}
			err := GetObject(v1alpha2.VirtualMachineResource, vmName, &vm, kc.GetOptions{Namespace: namespace})
			if err != nil {
				return fmt.Errorf("virtualMachine: %s\nstderr: %w", vmName, err)
			}
			for _, bd := range vm.Status.BlockDeviceRefs {
				if !bd.Attached {
					return fmt.Errorf("virtualMachine: %s\nblockDeviceRefs: %#v", vmName, bd)
				}
			}
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
}

func ResizeDisks(addedSize *resource.Quantity, config *cfg.Config, ns string, virtualDisks ...string) {
	GinkgoHelper()
	wg := &sync.WaitGroup{}
	for _, vd := range virtualDisks {
		wg.Add(1)
		go func() {
			defer GinkgoRecover()
			defer wg.Done()
			diskObject := v1alpha2.VirtualDisk{}
			err := GetObject(kc.ResourceVD, vd, &diskObject, kc.GetOptions{Namespace: ns})
			Expect(err).NotTo(HaveOccurred(), "%v", err)
			newValue := resource.NewQuantity(diskObject.Spec.PersistentVolumeClaim.Size.Value()+addedSize.Value(), resource.BinarySI)
			mergePatch := fmt.Sprintf("{\"spec\":{\"persistentVolumeClaim\":{\"size\":\"%s\"}}}", newValue.String())
			err = MergePatchResource(kc.ResourceVD, ns, vd, mergePatch)
			Expect(err).NotTo(HaveOccurred(), "%v", err)
		}()
	}
	wg.Wait()
}

func GetSizeFromObject(vdName, namespace string) (*resource.Quantity, error) {
	GinkgoHelper()
	vd := v1alpha2.VirtualDisk{}
	err := GetObject(kc.ResourceVD, vdName, &vd, kc.GetOptions{Namespace: namespace})
	if err != nil {
		return nil, err
	}
	return vd.Spec.PersistentVolumeClaim.Size, nil
}

func GetSizeByLsblk(vmNamespace, vmName, diskIDPath string) (*resource.Quantity, error) {
	GinkgoHelper()
	var (
		blockDevices *BlockDevices
		quantity     resource.Quantity
	)
	cmd := fmt.Sprintf("lsblk --json --nodeps --output size %s", diskIDPath)
	res := framework.GetClients().D8Virtualization().SSHCommand(vmName, cmd, d8.SSHOptions{
		Namespace:    vmNamespace,
		Username:     conf.TestData.SSHUser,
		IdentityFile: conf.TestData.Sshkey,
	})
	if res.Error() != nil {
		return nil, errors.New(res.StdErr())
	}
	err := json.Unmarshal(res.StdOutBytes(), &blockDevices)
	if err != nil {
		return nil, err
	}
	if len(blockDevices.BlockDevices) != 1 {
		return nil, fmt.Errorf("`blockDevices` length should be 1")
	}
	blockDevice := &blockDevices.BlockDevices[0]
	quantity = resource.MustParse(blockDevice.Size)
	return &quantity, nil
}

func GetDiskSize(vmNamespace, vmName, vdName, diskIDPath string, disk *DiskMetaData) {
	GinkgoHelper()
	sizeFromObject, err := GetSizeFromObject(vdName, vmNamespace)
	Expect(err).NotTo(HaveOccurred(), "%v", err)
	var sizeByLsblk *resource.Quantity
	Eventually(func() error {
		sizeByLsblk, err = GetSizeByLsblk(vmNamespace, vmName, diskIDPath)
		if err != nil {
			return fmt.Errorf("virtualMachine: %s\nstderr: %w", vmName, err)
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
	Expect(sizeFromObject).NotTo(BeNil())
	Expect(sizeByLsblk).NotTo(BeNil())
	disk.SizeFromObject = sizeFromObject
	disk.SizeByLsblk = sizeByLsblk
}

func GetDiskIDPath(vdName string, vmi *virtv1.VirtualMachineInstance) string {
	diskName := fmt.Sprintf("vd-%s", vdName)
	for _, disk := range vmi.Spec.Domain.Devices.Disks {
		if disk.Name == diskName {
			return fmt.Sprintf("/dev/disk/by-id/%s_%s", DiskIDPrefix, disk.Serial)
		}
	}
	return ""
}

// Refactor this flow when `target` field will be fixed for `VirtualMachine.Status.BlockDeviceRefs`
func GetVirtualMachineDisks(vmNamespace, vmName string) (VirtualMachineDisks, error) {
	GinkgoHelper()
	var vmObject v1alpha2.VirtualMachine
	disks := make(map[string]DiskMetaData, 0)
	err := GetObject(v1alpha2.VirtualMachineResource, vmName, &vmObject, kc.GetOptions{
		Namespace: vmNamespace,
	})
	if err != nil {
		return disks, err
	}

	intVirtVmi := &virtv1.VirtualMachineInstance{}
	err = GetObject(kc.ResourceKubevirtVMI, vmName, intVirtVmi, kc.GetOptions{
		Namespace: vmNamespace,
	})
	if err != nil {
		return disks, err
	}

	for _, device := range vmObject.Status.BlockDeviceRefs {
		disk := DiskMetaData{}
		if device.Kind != v1alpha2.DiskDevice {
			continue
		}
		diskIDPath := GetDiskIDPath(device.Name, intVirtVmi)
		GetDiskSize(vmNamespace, vmName, device.Name, diskIDPath, &disk)
		disks[device.Name] = disk
	}
	return disks, nil
}
