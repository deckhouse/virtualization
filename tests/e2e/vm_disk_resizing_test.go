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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	cfg "github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

type VirtualMachineDisks map[string]DiskMetaData

type DiskMetaData struct {
	Id             string
	SizeByLsblk    *resource.Quantity
	SizeFromObject *resource.Quantity
}

const (
	DiskIdPrefix  = "scsi-0QEMU_QEMU_HARDDISK"
	CdRomIdPrefix = "scsi-0QEMU_QEMU_CD-ROM_drive-ua"
)

func WaitBlockDeviceRefsAttached(namespace string, vms ...string) {
	GinkgoHelper()
	Eventually(func() error {
		for _, vmName := range vms {
			vm := virtv2.VirtualMachine{}
			err := GetObject(virtv2.VirtualMachineResource, vmName, &vm, kc.GetOptions{Namespace: namespace})
			if err != nil {
				return fmt.Errorf("virtualMachine: %s\nstderr: ws", vmName, err)
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

func ResizeDisks(addedSize *resource.Quantity, config *cfg.Config, virtualDisks ...string) {
	GinkgoHelper()
	wg := &sync.WaitGroup{}
	for _, vd := range virtualDisks {
		wg.Add(1)
		go func() {
			defer GinkgoRecover()
			defer wg.Done()
			diskObject := virtv2.VirtualDisk{}
			err := GetObject(kc.ResourceVD, vd, &diskObject, kc.GetOptions{Namespace: config.Namespace})
			Expect(err).NotTo(HaveOccurred(), "%v", err)
			newValue := resource.NewQuantity(diskObject.Spec.PersistentVolumeClaim.Size.Value()+addedSize.Value(), resource.BinarySI)
			mergePatch := fmt.Sprintf("{\"spec\":{\"persistentVolumeClaim\":{\"size\":\"%s\"}}}", newValue.String())
			err = MergePatchResource(kc.ResourceVD, vd, mergePatch)
			Expect(err).NotTo(HaveOccurred(), "%v", err)
		}()
	}
	wg.Wait()
}

func GetSizeFromObject(vdName, namespace string) (*resource.Quantity, error) {
	GinkgoHelper()
	vd := virtv2.VirtualDisk{}
	err := GetObject(kc.ResourceVD, vdName, &vd, kc.GetOptions{Namespace: namespace})
	if err != nil {
		return nil, err
	}
	return vd.Spec.PersistentVolumeClaim.Size, nil
}

func GetSizeByLsblk(vmName, diskIdPath string) (*resource.Quantity, error) {
	GinkgoHelper()
	var (
		blockDevices *BlockDevices
		quantity     resource.Quantity
	)
	cmd := fmt.Sprintf("lsblk --json --nodeps --output size %s", diskIdPath)
	res := d8Virtualization.SSHCommand(vmName, cmd, d8.SSHOptions{
		Namespace:   conf.Namespace,
		Username:    conf.TestData.SSHUser,
		IdenityFile: conf.TestData.Sshkey,
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

func GetDiskSize(vmName, vdName, diskIdPath string, config *cfg.Config, disk *DiskMetaData) {
	GinkgoHelper()
	sizeFromObject, err := GetSizeFromObject(vdName, config.Namespace)
	Expect(err).NotTo(HaveOccurred(), "%v", err)
	var sizeByLsblk *resource.Quantity
	Eventually(func() error {
		sizeByLsblk, err = GetSizeByLsblk(vmName, diskIdPath)
		if err != nil {
			return fmt.Errorf("virtualMachine: %s\nstderr: ws", vmName, err)
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
	Expect(sizeFromObject).NotTo(BeNil())
	Expect(sizeByLsblk).NotTo(BeNil())
	disk.SizeFromObject = sizeFromObject
	disk.SizeByLsblk = sizeByLsblk
}

func GetDiskIdPath(vdName string, vmi *virtv1.VirtualMachineInstance) string {
	diskName := fmt.Sprintf("vd-%s", vdName)
	for _, disk := range vmi.Spec.Domain.Devices.Disks {
		if disk.Name == diskName {
			return fmt.Sprintf("/dev/disk/by-id/%s_%s", DiskIdPrefix, disk.Serial)
		}
	}
	return ""
}

// Refactor this flow when `target` field will be fixed for `VirtualMachine.Status.BlockDeviceRefs`
func GetVirtualMachineDisks(vmName string, config *cfg.Config) (VirtualMachineDisks, error) {
	GinkgoHelper()
	var vmObject virtv2.VirtualMachine
	disks := make(map[string]DiskMetaData, 0)
	err := GetObject(virtv2.VirtualMachineResource, vmName, &vmObject, kc.GetOptions{
		Namespace: config.Namespace,
	})
	if err != nil {
		return disks, err
	}

	intVirtVmi := &virtv1.VirtualMachineInstance{}
	err = GetObject(kc.ResourceKubevirtVMI, vmName, intVirtVmi, kc.GetOptions{
		Namespace: config.Namespace,
	})
	if err != nil {
		return disks, err
	}

	for _, device := range vmObject.Status.BlockDeviceRefs {
		disk := DiskMetaData{}
		if device.Kind != virtv2.DiskDevice {
			continue
		}
		diskIdPath := GetDiskIdPath(device.Name, intVirtVmi)
		GetDiskSize(vmName, device.Name, diskIdPath, config, &disk)
		disks[device.Name] = disk
	}
	return disks, nil
}

var _ = Describe("Virtual disk resizing", ginkgoutil.CommonE2ETestDecorators(), func() {
	const (
		vmCount   = 1
		diskCount = 3
	)
	var vmObj *virtv2.VirtualMachine
	testCaseLabel := map[string]string{"testcase": "disk-resizing"}

	BeforeAll(func() {
		if cfg.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}

		kustomization := fmt.Sprintf("%s/%s", conf.TestData.DiskResizing, "kustomization.yaml")
		ns, err := kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
		conf.SetNamespace(ns)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
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
			By(fmt.Sprintf("`VirtualImages` should be in the %q phases", virtv2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceVI, string(virtv2.ImageReady), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the virtual disks are applied", func() {
		It("checks `VirtualDisks` phase", func() {
			By(fmt.Sprintf("`VirtualDisks` should be in the %q phases", virtv2.DiskReady), func() {
				WaitPhaseByLabel(kc.ResourceVD, string(virtv2.DiskReady), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the virtual machine are applied", func() {
		It("checks `VirtualMachine` phase", func() {
			By("`VirtualMachine` agent should be ready", func() {
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		It("retrieves the test objects", func() {
			By("`VirtualMachine`", func() {
				vmObjs := &virtv2.VirtualMachineList{}
				err := GetObjects(virtv2.VirtualMachineResource, vmObjs, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
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
			By(fmt.Sprintf("`VirtualMachineBlockDeviceAttachment` should be in the %q phases", virtv2.BlockDeviceAttachmentPhaseAttached), func() {
				WaitPhaseByLabel(kc.ResourceVMBDA, string(virtv2.BlockDeviceAttachmentPhaseAttached), kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
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
				vmDisksBefore, err = GetVirtualMachineDisks(vmObj.Name, conf)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmDisksBefore).Should(HaveLen(diskCount))
			})

			It("resizes the disks", func() {
				wg := &sync.WaitGroup{}
				res := kubectl.List(kc.ResourceVD, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
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
					By(fmt.Sprintf("`VirtualDisks` should be in the %q phase", virtv2.DiskResizing), func() {
						WaitPhaseByLabel(virtv2.VirtualDiskResource, string(virtv2.DiskResizing), kc.WaitOptions{
							Labels:    testCaseLabel,
							Namespace: conf.Namespace,
							Timeout:   MaxWaitTimeout,
						})
					})
				}()
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					ResizeDisks(addedSize, conf, vds...)
				}()
				wg.Wait()
			})

			It("checks `VirtualDisks`, `VirtualMachine` and `VirtualMachineBlockDeviceAttachment` phases", func() {
				By(fmt.Sprintf("`VirtualDisks` should be in the %q phases", virtv2.DiskReady), func() {
					WaitPhaseByLabel(kc.ResourceVD, string(virtv2.DiskReady), kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: conf.Namespace,
						Timeout:   MaxWaitTimeout,
					})
				})
				By("`VirtualMachine` should be ready", func() {
					WaitVmAgentReady(kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: conf.Namespace,
						Timeout:   MaxWaitTimeout,
					})
				})
				By(fmt.Sprintf("`VirtualMachineBlockDeviceAttachment` should be in the %q phases", virtv2.BlockDeviceAttachmentPhaseAttached), func() {
					WaitPhaseByLabel(kc.ResourceVMBDA, string(virtv2.BlockDeviceAttachmentPhaseAttached), kc.WaitOptions{
						Labels:    testCaseLabel,
						Namespace: conf.Namespace,
						Timeout:   MaxWaitTimeout,
					})
				})
				By("`BlockDevices` from the status should be attached", func() {
					WaitBlockDeviceRefsAttached(conf.Namespace, vmObj.Name)
				})
			})

			It("obtains and compares the disks metadata after resizing", func() {
				Eventually(func() error {
					vmDisksAfter, err = GetVirtualMachineDisks(vmObj.Name, conf)
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
			DeleteTestCaseResources(ResourcesToDelete{
				KustomizationDir: conf.TestData.DiskResizing,
			})
		})
	})
})
