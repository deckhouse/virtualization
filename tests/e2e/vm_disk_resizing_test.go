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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"

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

const DiskIdPrefix = "scsi-0QEMU_QEMU_HARDDISK_"

func WaitBlockDeviceRefsStatus(namespace string, vms ...string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, vmName := range vms {
			vm := virtv2.VirtualMachine{}
			err := GetObject(kc.ResourceKubevirtVM, vmName, &vm, kc.GetOptions{Namespace: namespace})
			Expect(err).NotTo(HaveOccurred(), err)
			for _, disk := range vm.Status.BlockDeviceRefs {
				Expect(disk.Attached).To(BeTrue(), "attached status check failed: %#v", vm.Status.BlockDeviceRefs)
			}
		}
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
}

func ResizeDisks(addedSize *resource.Quantity, config *cfg.Config, virtualDisks ...string) {
	GinkgoHelper()
	for _, vd := range virtualDisks {
		diskObject := virtv2.VirtualDisk{}
		err := GetObject(kc.ResourceVD, vd, &diskObject, kc.GetOptions{Namespace: config.Namespace})
		Expect(err).NotTo(HaveOccurred(), err)
		newValue := resource.NewQuantity(diskObject.Spec.PersistentVolumeClaim.Size.Value()+addedSize.Value(), resource.BinarySI)
		mergePatch := fmt.Sprintf("{\"spec\":{\"persistentVolumeClaim\":{\"size\":\"%s\"}}}", newValue.String())
		err = MergePatchResource(kc.ResourceVD, vd, mergePatch)
		Expect(err).NotTo(HaveOccurred(), err)
	}
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

func GetSizeByLsblk(vmName, diskId string) (*resource.Quantity, error) {
	GinkgoHelper()
	var (
		blockDevice BlockDevice
		quantity    resource.Quantity
	)
	cmd := fmt.Sprintf("lsblk --json --output size %s", diskId)
	res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
		Namespace:   conf.Namespace,
		Username:    conf.TestData.SshUser,
		IdenityFile: conf.TestData.Sshkey,
	})
	if res.Error() != nil {
		return nil, fmt.Errorf(res.StdErr())
	}
	err := json.Unmarshal(res.StdOutBytes(), &blockDevice)
	if err != nil {
		return nil, err
	}
	quantity = resource.MustParse(blockDevice.Size)
	return &quantity, nil
}

func GetDiskSize(vmName, vdName, diskId string, config *cfg.Config, disk *DiskMetaData) {
	GinkgoHelper()
	sizeFromObject, err := GetSizeFromObject(vdName, config.Namespace)
	Expect(err).NotTo(HaveOccurred(), err)
	var sizeByLsblk *resource.Quantity
	Eventually(func(g Gomega) {
		sizeByLsblk, err = GetSizeByLsblk(vmName, diskId)
		Expect(err).NotTo(HaveOccurred(), err)
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
	disk.SizeFromObject = sizeFromObject
	disk.SizeByLsblk = sizeByLsblk
}

// Refactor this flow when `target` field will be fixed for `VirtualMachine.Status.BlockDeviceRefs`
func GetVirtualMachineDisks(vmName string, config *cfg.Config) (VirtualMachineDisks, error) {
	GinkgoHelper()
	var vmObject virtv2.VirtualMachine
	disks := make(map[string]DiskMetaData, 0)
	err := GetObject(kc.ResourceKubevirtVM, vmName, &vmObject, kc.GetOptions{Namespace: config.Namespace})
	if err != nil {
		return disks, err
	}

	for _, device := range vmObject.Spec.BlockDeviceRefs {
		disk := DiskMetaData{}
		if device.Kind != virtv2.DiskDevice {
			continue
		}
		diskId := fmt.Sprintf("%s-vd-%s", DiskIdPrefix, device.Name)
		GetDiskSize(vmName, device.Name, diskId, config, &disk)
		disks[device.Name] = disk
	}

	for _, device := range vmObject.Status.BlockDeviceRefs {
		disk := DiskMetaData{}
		if device.Kind != virtv2.DiskDevice {
			continue
		}
		if _, found := disks[device.Name]; found {
			continue
		}
		diskId := fmt.Sprintf("%s-%s", DiskIdPrefix, device.Name)
		GetDiskSize(vmName, device.Name, diskId, config, &disk)
		disks[device.Name] = disk
	}
	return disks, nil
}

var _ = Describe("Virtual disk resizing", ginkgoutil.CommonE2ETestDecorators(), func() {
	diskResizingLabel := map[string]string{"testcase": "disk-resizing"}

	Context("When resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Kustomize(conf.TestData.DiskResizing, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				Labels:    diskResizingLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    diskResizingLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machine block device attachments are applied:", func() {
		It("checks VMBDAs phases", func() {
			By(fmt.Sprintf("VMBDAs should be in %s phases", PhaseAttached))
			WaitPhase(kc.ResourceVMBDA, PhaseAttached, kc.GetOptions{
				Labels:    diskResizingLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Describe("Resizing", func() {
		Context(fmt.Sprintf("When virtual machines are in %s phases:", PhaseRunning), func() {
			var (
				vmDisksBefore VirtualMachineDisks
				vmDisksAfter  VirtualMachineDisks
				err           error
			)
			vmName := fmt.Sprintf("%s-vm-%s", namePrefix, diskResizingLabel["testcase"])
			It("get disks metadata before resizing", func() {
				vmDisksBefore, err = GetVirtualMachineDisks(vmName, conf)
				Expect(err).NotTo(HaveOccurred(), err)
			})
			It("resizes disks", func() {
				res := kubectl.List(kc.ResourceVD, kc.GetOptions{
					Labels:    diskResizingLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

				vds := strings.Split(res.StdOut(), " ")
				addedSize := resource.NewQuantity(100*1024*1024, resource.BinarySI)
				ResizeDisks(addedSize, conf, vds...)
			})
			It("get disks metadata after resizing", func() {
				vmDisksAfter, err = GetVirtualMachineDisks(vmName, conf)
				Expect(err).NotTo(HaveOccurred(), err)
			})

			It("checks VDs, VMs and VMBDA phases", func() {
				By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
				WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
					Labels:    diskResizingLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})

				By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
				WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
					Labels:    diskResizingLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})

				By(fmt.Sprintf("VMBDAs should be in %s phases", PhaseAttached))
				WaitPhase(kc.ResourceVMBDA, PhaseAttached, kc.GetOptions{
					Labels:    diskResizingLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})

				By("BlockDeviceRefsStatus: disks should be attached")
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    diskResizingLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
				vms := strings.Split(res.StdOut(), " ")
				WaitBlockDeviceRefsStatus(conf.Namespace, vms...)
			})

			It(fmt.Sprintf("compares disk size before and after resizing for %s", vmName), func() {
				for disk := range vmDisksBefore {
					By(fmt.Sprintf("Compare disks after resizing: %s", disk))
					sizeFromObjectBefore := vmDisksBefore[disk].SizeFromObject.Value()
					sizeFromObjectAfter := vmDisksAfter[disk].SizeFromObject.Value()
					compareSizeFromObjects := sizeFromObjectBefore < sizeFromObjectAfter
					Expect(compareSizeFromObjects).To(BeTrue(), "size from objects before must be lower than size after: before: %d, after: %d", sizeFromObjectBefore, sizeFromObjectAfter)
					sizeByLsblkBefore := vmDisksBefore[disk].SizeByLsblk.Value()
					sizeByLsblkAfter := vmDisksAfter[disk].SizeByLsblk.Value()
					compareSizeByLsblk := sizeByLsblkBefore < sizeByLsblkAfter
					Expect(compareSizeByLsblk).To(BeTrue(), "size by lsblk before must be lower than size after: before: %d, after: %d", sizeByLsblkBefore, sizeByLsblkAfter)
				}
			})
		})
	})
})
