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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

type Image struct {
	Kind string
	Name string
}

func IsBlockDeviceCdRom(vmName, blockDeviceName string) (bool, error) {
	var blockDevices *BlockDevices
	bdIdPath := fmt.Sprintf("/dev/disk/by-id/%s-%s", CdRomIdPrefix, blockDeviceName)
	cmd := fmt.Sprintf("lsblk --json --nodeps --output name,type %s", bdIdPath)
	res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
		Namespace:   conf.Namespace,
		Username:    conf.TestData.SSHUser,
		IdenityFile: conf.TestData.Sshkey,
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

func MountBlockDevice(vmName, blockDeviceId string) error {
	bdIdPath := fmt.Sprintf("/dev/disk/by-id/%s", blockDeviceId)
	cmd := fmt.Sprintf("sudo mount --read-only %s /mnt", bdIdPath)
	res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
		Namespace:   conf.Namespace,
		Username:    conf.TestData.SSHUser,
		IdenityFile: conf.TestData.Sshkey,
	})
	if res.Error() != nil {
		return errors.New(res.StdErr())
	}
	return nil
}

func IsBlockDeviceReadOnly(vmName, blockDeviceId string) (bool, error) {
	bdIdPath := fmt.Sprintf("/dev/disk/by-id/%s", blockDeviceId)
	cmd := fmt.Sprintf("findmnt --noheadings --output options %s", bdIdPath)
	res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
		Namespace:   conf.Namespace,
		Username:    conf.TestData.SSHUser,
		IdenityFile: conf.TestData.Sshkey,
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

type Counter struct {
	Current  int
	Expected int
}

type ReusableResources map[kc.Resource]*Counter

// Useful when require to check the created resources in `REUSABLE` mode.
//
//	Static output option: `jsonpath='{.items[*].metadata.name}'`.
func CheckReusableResources(resources ReusableResources, opts kc.GetOptions) {
	GinkgoHelper()
	opts.Output = "jsonpath='{.items[*].metadata.name}'"
	for r, c := range resources {
		res := kubectl.List(r, opts)
		Expect(res.Error()).NotTo(HaveOccurred(), "failed to check the reusable resources %q: %s", r, res.StdErr())
		c.Current = len(strings.Split(res.StdOut(), " "))
	}

	isReusableResourcesExist := false
	for _, c := range resources {
		if c.Current == c.Expected {
			isReusableResourcesExist = true
		} else {
			isReusableResourcesExist = false
		}
	}
	if isReusableResourcesExist {
		return
	}
}

var _ = Describe("Image hotplug", ginkgoutil.CommonE2ETestDecorators(), func() {
	const (
		viCount    = 2
		cviCount   = 2
		vmCount    = 1
		vdCount    = 1
		vmbdaCount = 0
		imgCount   = viCount + cviCount
	)

	var (
		vmObj         virtv2.VirtualMachine
		disksBefore   Disks
		disksAfter    Disks
		testCaseLabel = map[string]string{"testcase": "image-hotplug"}
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.ImageHotplug, "kustomization.yaml")
		ns, err := kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
		conf.SetNamespace(ns)

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
					virtv2.ClusterVirtualImageResource: &Counter{
						Expected: cviCount,
					},
					virtv2.VirtualMachineBlockDeviceAttachmentResource: &Counter{
						Expected: vmbdaCount,
					},
				}, kc.GetOptions{
					Labels:         testCaseLabel,
					Namespace:      conf.Namespace,
					IgnoreNotFound: true,
				})
			}

			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.ImageHotplug},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})

		It("checks the resources phase", func() {
			By(fmt.Sprintf("`VirtualImages` should be in the %q phase", virtv2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			By(fmt.Sprintf("`ClusterVirtualImages` should be in the %q phase", virtv2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceCVI, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			By(fmt.Sprintf("`VirtualDisk` should be in the %q phase", virtv2.DiskReady), func() {
				WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			By("`VirtualMachine` agent should be ready", func() {
				WaitVmAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the resources are ready to use", func() {
		imageBlockDevices := make([]Image, 0, imgCount)

		It("retrieves the test objects", func() {
			By("`VirtualMachine`", func() {
				vmObjs := &virtv2.VirtualMachineList{}
				err := GetObjects(virtv2.VirtualMachineResource, vmObjs, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to get `VirtualMachines`: %s", err)
				Expect(len(vmObjs.Items)).To(Equal(vmCount), "there is only %d `VirtualMachine` in this test case", vmCount)
				vmObj = vmObjs.Items[0]
			})
			By("`VirtualImages`", func() {
				viObjs := &virtv2.VirtualImageList{}
				err := GetObjects(virtv2.VirtualImageResource, viObjs, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
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
				cviObjs := &virtv2.ClusterVirtualImageList{}
				err := GetObjects(virtv2.ClusterVirtualImageResource, cviObjs, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
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
				return GetDisksMetadata(vmObj.Name, &disksBefore)
			}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred(), "virtualMachine: %s", vmObj.Name)
		})

		It("attaches the images into the `VirtualMachine`", func() {
			for _, bd := range imageBlockDevices {
				By(bd.Name, func() {
					AttachBlockDevice(vmObj.Name, bd.Name, virtv2.VMBDAObjectRefKind(bd.Kind), testCaseLabel, conf.TestData.ImageHotplug)
				})
			}
		})

		It("checks the `VirtualMachine` and the `VirtualMachineBlockDeviceAttachments` phases", func() {
			By(fmt.Sprintf("`VirtualMachineBlockDeviceAttachments` should be in the %q phase", virtv2.BlockDeviceAttachmentPhaseAttached), func() {
				WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
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
			By("`BlockDevices` should be attached", func() {
				WaitBlockDeviceRefsAttached(conf.Namespace, vmObj.Name)
			})
		})

		It("compares the disk count before and after attachment", func() {
			diskCountBefore := len(disksBefore.BlockDevices)
			Eventually(func() (int, error) {
				err := GetDisksMetadata(vmObj.Name, &disksAfter)
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
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to get `InternalVirtualMachineInstance`: %s", err)
			for _, disk := range intVirtVmi.Spec.Domain.Devices.Disks {
				if disk.CDRom != nil {
					isoBlockDeviceName = disk.Name
					isolockDeviceCount += 1
				}
			}
			Expect(isolockDeviceCount).To(Equal(1), "there is only one `ISO` block device in this case")
			isCdRom, err := IsBlockDeviceCdRom(vmObj.Name, isoBlockDeviceName)
			Expect(err).NotTo(HaveOccurred(), "failed to get `BlockDeviceType` of %q: %s", isoBlockDeviceName, err)
			Expect(isCdRom).Should(BeTrue(), "wrong type of the block device: %s", isoBlockDeviceName)
		})

		It("check that the images are attached as the `ReadOnly` devices", func() {
			imgs := make(map[string]string, imgCount)
			intVirtVmi := &virtv1.VirtualMachineInstance{}
			err := GetObject(kc.ResourceKubevirtVMI, vmObj.Name, intVirtVmi, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to get `InternalVirtulMachineInstance`: %s", err)
			for _, disk := range intVirtVmi.Spec.Domain.Devices.Disks {
				switch {
				case strings.HasSuffix(disk.Name, "iso"):
					imgs[disk.Name] = fmt.Sprintf("%s-%s", CdRomIdPrefix, disk.Name)
				case strings.HasPrefix(disk.Name, "cvi-") || strings.HasPrefix(disk.Name, "vi-"):
					imgs[disk.Name] = fmt.Sprintf("%s_%s", DiskIdPrefix, disk.Serial)
				}
			}

			Expect(len(imgs)).To(Equal(imgCount), "there are only %d `blockDevices` in this case", imgCount)
			for img, diskId := range imgs {
				err := MountBlockDevice(vmObj.Name, diskId)
				Expect(err).NotTo(HaveOccurred(), "failed to mount %q into the `VirtualMachine`: %s", img, err)
				isReadOnly, err := IsBlockDeviceReadOnly(vmObj.Name, diskId)
				Expect(err).NotTo(HaveOccurred(), "failed to check the `ReadOnly` status: %s", img)
				Expect(isReadOnly).Should(BeTrue(), "the mounted disk should be `ReadOnly`")
			}
		})

		It("detaches the images", func() {
			res := kubectl.Delete(kc.DeleteOptions{
				FilenameOption: kc.Filename,
				Filename:       []string{fmt.Sprintf("%s/vmbda", conf.TestData.ImageHotplug)},
				Namespace:      conf.Namespace,
				Labels:         testCaseLabel,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "failed to delete `VirtualMachineBlockDeviceAttachments`: %s", res.StdErr())
		})

		It("compares the disk count after detachment", func() {
			diskCountBefore := len(disksBefore.BlockDevices)
			Expect(diskCountBefore).NotTo(BeZero(), "the disk count `before` should not be zero")
			Eventually(func() (int, error) {
				err := GetDisksMetadata(vmObj.Name, &disksAfter)
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

			DeleteTestCaseResources(resourcesToDelete)
		})
	})
})
