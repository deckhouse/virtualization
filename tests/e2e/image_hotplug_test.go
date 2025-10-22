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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("ImageHotplug", framework.CommonE2ETestDecorators(), func() {
	const (
		viCount  = 2
		cviCount = 2
		vmCount  = 1
		imgCount = viCount + cviCount
	)

	var (
		vmObj         v1alpha2.VirtualMachine
		disksBefore   Disks
		disksAfter    Disks
		testCaseLabel = map[string]string{"testcase": "image-hotplug"}
		ns            string
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.ImageHotplug, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		res := kubectl.Delete(kc.DeleteOptions{
			IgnoreNotFound: true,
			Labels:         testCaseLabel,
			Resource:       kc.ResourceCVI,
		})
		Expect(res.Error()).NotTo(HaveOccurred())

		CreateNamespace(ns)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
	})

	Context("When the virtualization resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.ImageHotplug},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})

		It("checks the resources phase", func() {
			By(fmt.Sprintf("`VirtualImages` should be in the %q phase", v1alpha2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			By(fmt.Sprintf("`ClusterVirtualImages` should be in the %q phase", v1alpha2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceCVI, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			By(fmt.Sprintf("`VirtualDisk` should be in the %q phase", v1alpha2.DiskReady), func() {
				WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			By("`VirtualMachine` agent should be ready", func() {
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the resources are ready to use", func() {
		imageBlockDevices := make([]Image, 0, imgCount)

		It("retrieves the test objects", func() {
			By("`VirtualMachine`", func() {
				vmObjs := &v1alpha2.VirtualMachineList{}
				err := GetObjects(v1alpha2.VirtualMachineResource, vmObjs, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to get `VirtualMachines`: %s", err)
				Expect(len(vmObjs.Items)).To(Equal(vmCount), "there is only %d `VirtualMachine` in this test case", vmCount)
				vmObj = vmObjs.Items[0]
			})
			By("`VirtualImages`", func() {
				viObjs := &v1alpha2.VirtualImageList{}
				err := GetObjects(v1alpha2.VirtualImageResource, viObjs, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to get `VirtualImages`: %s", err)

				for _, viObj := range viObjs.Items {
					imageBlockDevices = append(imageBlockDevices, Image{
						Kind: viObj.Kind,
						Name: viObj.Name,
					})
				}
			})
			By("`ClusterVirtualImages`", func() {
				cviObjs := &v1alpha2.ClusterVirtualImageList{}
				err := GetObjects(v1alpha2.ClusterVirtualImageResource, cviObjs, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to get `ClusterVirtualImages`: %s", err)

				for _, cviObj := range cviObjs.Items {
					imageBlockDevices = append(imageBlockDevices, Image{
						Kind: cviObj.Kind,
						Name: cviObj.Name,
					})
				}
			})
		})

		It("retrieves the disk count before the images attachment", func() {
			Eventually(func() error {
				return GetDisksMetadata(ns, vmObj.Name, &disksBefore)
			}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred(), "virtualMachine: %s", vmObj.Name)
		})

		It("attaches the images into the `VirtualMachine`", func() {
			for _, bd := range imageBlockDevices {
				By(bd.Name, func() {
					AttachBlockDevice(ns, vmObj.Name, bd.Name, v1alpha2.VMBDAObjectRefKind(bd.Kind), testCaseLabel, conf.TestData.ImageHotplug)
				})
			}
		})

		It("checks the `VirtualMachine` and the `VirtualMachineBlockDeviceAttachments` phases", func() {
			By(fmt.Sprintf("`VirtualMachineBlockDeviceAttachments` should be in the %q phase", v1alpha2.BlockDeviceAttachmentPhaseAttached), func() {
				WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
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
			By("`BlockDevices` should be attached", func() {
				WaitBlockDeviceRefsAttached(ns, vmObj.Name)
			})
		})

		It("compares the disk count before and after attachment", func() {
			diskCountBefore := len(disksBefore.BlockDevices)
			Eventually(func() (int, error) {
				err := GetDisksMetadata(ns, vmObj.Name, &disksAfter)
				if err != nil {
					return unacceptableCount, err
				}
				diskCountAfter := len(disksAfter.BlockDevices)
				return diskCountAfter, nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(diskCountBefore+imgCount), "comparing error: 'after' must be equal 'before + %d'", imgCount)
		})

		It("checks that the `ISO` image is attached as `CD-ROM`", func() {
			var (
				isoBlockDeviceName string
				isolockDeviceCount int
			)
			intVirtVmi := &virtv1.VirtualMachineInstance{}
			err := GetObject(kc.ResourceKubevirtVMI, vmObj.Name, intVirtVmi, kc.GetOptions{
				Namespace: ns,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to get `InternalVirtualMachineInstance`: %s", err)
			for _, disk := range intVirtVmi.Spec.Domain.Devices.Disks {
				if disk.CDRom != nil {
					isoBlockDeviceName = disk.Name
					isolockDeviceCount += 1
				}
			}
			Expect(isolockDeviceCount).To(Equal(1), "there is only one `ISO` block device in this case")
			isCdRom, err := IsBlockDeviceCdRom(ns, vmObj.Name, isoBlockDeviceName)
			Expect(err).NotTo(HaveOccurred(), "failed to get `BlockDeviceType` of %q: %s", isoBlockDeviceName, err)
			Expect(isCdRom).Should(BeTrue(), "wrong type of the block device: %s", isoBlockDeviceName)
		})

		It("check that the images are attached as the `ReadOnly` devices", func() {
			imgs := make(map[string]string, imgCount)
			intVirtVmi := &virtv1.VirtualMachineInstance{}
			err := GetObject(kc.ResourceKubevirtVMI, vmObj.Name, intVirtVmi, kc.GetOptions{
				Namespace: ns,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to get `InternalVirtulMachineInstance`: %s", err)
			for _, disk := range intVirtVmi.Spec.Domain.Devices.Disks {
				switch {
				case strings.HasSuffix(disk.Name, "iso"):
					imgs[disk.Name] = fmt.Sprintf("%s-%s", CdRomIDPrefix, disk.Name)
				case strings.HasPrefix(disk.Name, "cvi-") || strings.HasPrefix(disk.Name, "vi-"):
					imgs[disk.Name] = fmt.Sprintf("%s_%s", DiskIDPrefix, disk.Serial)
				}
			}

			Expect(len(imgs)).To(Equal(imgCount), "there are only %d `blockDevices` in this case", imgCount)
			for img, diskID := range imgs {
				err := MountBlockDevice(ns, vmObj.Name, diskID)
				Expect(err).NotTo(HaveOccurred(), "failed to mount %q into the `VirtualMachine`: %s", img, err)
				isReadOnly, err := IsBlockDeviceReadOnly(ns, vmObj.Name, diskID)
				Expect(err).NotTo(HaveOccurred(), "failed to check the `ReadOnly` status: %s", img)
				Expect(isReadOnly).Should(BeTrue(), "the mounted disk should be `ReadOnly`")
			}
		})

		It("detaches the images", func() {
			res := kubectl.Delete(kc.DeleteOptions{
				FilenameOption: kc.Filename,
				Filename:       []string{fmt.Sprintf("%s/vmbda", conf.TestData.ImageHotplug)},
				Namespace:      ns,
				Labels:         testCaseLabel,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "failed to delete `VirtualMachineBlockDeviceAttachments`: %s", res.StdErr())
		})

		It("compares the disk count after detachment", func() {
			diskCountBefore := len(disksBefore.BlockDevices)
			Expect(diskCountBefore).NotTo(BeZero(), "the disk count `before` should not be zero")
			Eventually(func() (int, error) {
				err := GetDisksMetadata(ns, vmObj.Name, &disksAfter)
				if err != nil {
					return unacceptableCount, err
				}
				diskCountAfter := len(disksAfter.BlockDevices)
				return diskCountAfter, nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(diskCountBefore), "comparing error: 'after' must be equal 'before'")
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			resourcesToDelete := ResourcesToDelete{
				AdditionalResources: []AdditionalResource{
					{
						kc.ResourceVMBDA,
						testCaseLabel,
					},
				},
			}

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.ImageHotplug
			}

			DeleteTestCaseResources(ns, resourcesToDelete)
		})
	})
})

type Image struct {
	Kind string
	Name string
}

func IsBlockDeviceCdRom(vmNamespace, vmName, blockDeviceName string) (bool, error) {
	var blockDevices *BlockDevices
	bdIDPath := fmt.Sprintf("/dev/disk/by-id/%s-%s", CdRomIDPrefix, blockDeviceName)
	cmd := fmt.Sprintf("lsblk --json --nodeps --output name,type %s", bdIDPath)
	res := framework.GetClients().D8Virtualization().SSHCommand(vmName, cmd, d8.SSHOptions{
		Namespace:    vmNamespace,
		Username:     conf.TestData.SSHUser,
		IdentityFile: conf.TestData.Sshkey,
	})
	if res.Error() != nil {
		return false, errors.New(res.StdErr())
	}
	err := json.Unmarshal(res.StdOutBytes(), &blockDevices)
	if err != nil {
		return false, err
	}
	if len(blockDevices.BlockDevices) != 1 {
		return false, fmt.Errorf("`blockDevices` length should be 1")
	}
	blockDevice := &blockDevices.BlockDevices[0]
	return blockDevice.Type == "rom", nil
}

func MountBlockDevice(vmNamespace, vmName, blockDeviceID string) error {
	bdIDPath := fmt.Sprintf("/dev/disk/by-id/%s", blockDeviceID)
	cmd := fmt.Sprintf("sudo mount --read-only %s /mnt", bdIDPath)
	res := framework.GetClients().D8Virtualization().SSHCommand(vmName, cmd, d8.SSHOptions{
		Namespace:    vmNamespace,
		Username:     conf.TestData.SSHUser,
		IdentityFile: conf.TestData.Sshkey,
	})
	if res.Error() != nil {
		return errors.New(res.StdErr())
	}
	return nil
}

func IsBlockDeviceReadOnly(vmNamespace, vmName, blockDeviceID string) (bool, error) {
	bdIDPath := fmt.Sprintf("/dev/disk/by-id/%s", blockDeviceID)
	cmd := fmt.Sprintf("findmnt --noheadings --output options %s", bdIDPath)
	res := framework.GetClients().D8Virtualization().SSHCommand(vmName, cmd, d8.SSHOptions{
		Namespace:    vmNamespace,
		Username:     conf.TestData.SSHUser,
		IdentityFile: conf.TestData.Sshkey,
	})
	if res.Error() != nil {
		return false, errors.New(res.StdErr())
	}
	options := strings.Split(res.StdOut(), ",")
	if len(options) == 0 {
		return false, fmt.Errorf("list of options is empty: %s", options)
	}
	roOpt := options[0]
	return roOpt == "ro", nil
}
