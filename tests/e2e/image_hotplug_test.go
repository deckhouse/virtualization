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

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	virtv1 "kubevirt.io/api/core/v1"
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
		Username:    conf.TestData.SshUser,
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
		Username:    conf.TestData.SshUser,
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
		Username:    conf.TestData.SshUser,
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

var _ = Describe("Image hotplug", func() {
	const (
		viCount    = 2
		cviCount   = 2
		vmCount    = 1
		vdCount    = 1
		vmbdaCount = 0
		imgCount   = viCount + cviCount
	)

	var (
		vmObj             virtv2.VirtualMachine
		disksBefore       Disks
		disksAfter        Disks
		testCaseLabel     = map[string]string{"testcase": "image-hotplug"}
		isoLabel          = "iso"
		imageBlockDevices = make([]Image, 0)
	)

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.ImageHotplug, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})
	})

	Context("When virtualization resources are applied", func() {
		It("result should be succeeded", func() {
			if config.IsReusable() {
				vms := &virtv2.VirtualMachineList{}
				err := GetObjects(virtv2.VirtualMachineResource, vms, kc.GetOptions{
					Labels:         testCaseLabel,
					Namespace:      conf.Namespace,
					IgnoreNotFound: true,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to check reusable `VirtualMachine`", err)

				vds := &virtv2.VirtualDiskList{}
				err = GetObjects(virtv2.VirtualDiskResource, vds, kc.GetOptions{
					Labels:         testCaseLabel,
					Namespace:      conf.Namespace,
					IgnoreNotFound: true,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to check reusable `VirtualDisk`", err)

				vis := &virtv2.VirtualImageList{}
				err = GetObjects(virtv2.VirtualImageResource, vis, kc.GetOptions{
					Labels:         testCaseLabel,
					Namespace:      conf.Namespace,
					IgnoreNotFound: true,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to check reusable `VirtualImages`", err)

				cvis := &virtv2.ClusterVirtualImageList{}
				err = GetObjects(virtv2.ClusterVirtualImageResource, cvis, kc.GetOptions{
					Labels:         testCaseLabel,
					IgnoreNotFound: true,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to check reusable `ClusterVirtualImages`", err)

				vmbdas := &virtv2.VirtualMachineBlockDeviceAttachmentList{}
				err = GetObjects(virtv2.VirtualMachineBlockDeviceAttachmentResource, vmbdas, kc.GetOptions{
					Labels:         testCaseLabel,
					IgnoreNotFound: true,
				})
				Expect(err).NotTo(HaveOccurred(), "failed to check reusable `VirtualMachineBlockDeviceAttachments`", err)

				if len(vms.Items) == vmCount &&
					len(vds.Items) == vdCount &&
					len(vis.Items) == viCount &&
					len(cvis.Items) == cviCount &&
					len(vmbdas.Items) == vmbdaCount {
					fmt.Println("all reusable resources have been found")
					return
				}

				if len(vms.Items) == 0 &&
					len(vds.Items) == 0 &&
					len(vis.Items) == 0 &&
					len(cvis.Items) == 0 &&
					len(vmbdas.Items) == 0 {
					fmt.Println("no found reusable resources in `REUSABLE` mode")
				}
			}

			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.ImageHotplug},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
		})
	})

	Context("When virtual images are applied", func() {
		It("checks the `VirtualImages` phase", func() {
			By(fmt.Sprintf("`VirtualImages` should be in the %q phase", virtv2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		It("retrieves `VirtualImages`", func() {
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
	})

	Context("When cluster virtual images are applied", func() {
		It("checks the `ClusterVirtualImages` phase", func() {
			By(fmt.Sprintf("`ClusterVirtualImages` should be in the %q phase", virtv2.ImageReady), func() {
				WaitPhaseByLabel(kc.ResourceCVI, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		It("retrieves `ClusterVirtualImages`", func() {
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

	Context("When the virtual disk is applied", func() {
		It("checks the `VirtualDisk` phase", func() {
			By(fmt.Sprintf("`VirtualDisk` should be in the %q phase", virtv2.DiskReady), func() {
				WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})
	})

	Context("When the virtual machine is applied", func() {
		It("checks the `VirtualMachine` status", func() {
			By("`VirtualMachine` should be ready", func() {
				WaitVmReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
		})

		It("retrieves `VirtualMachine` object", func() {
			vmObjs := &virtv2.VirtualMachineList{}
			err := GetObjects(virtv2.VirtualMachineResource, vmObjs, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to get `VirtualMachines`: %s", err)
			Expect(len(vmObjs.Items)).To(Equal(vmCount), "there is only %d `VirtualMachine` in this test case", vmCount)
			vmObj = vmObjs.Items[0]
		})
	})

	Context("When the virtual machine is ready", func() {
		It("retrieves the disk count before attachment", func() {
			Eventually(func() error {
				return GetDisksMetadata(vmObj.Name, &disksBefore)
			}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred(), "virtualMachine: %s", vmObj.Name)
		})

		It("attaches images into `VirtualMachine`", func() {
			for _, bd := range imageBlockDevices {
				AttachBlockDevice(vmObj.Name, bd.Name, virtv2.VMBDAObjectRefKind(bd.Kind), testCaseLabel, conf.TestData.ImageHotplug)
			}
		})

		It("checks `VirtualMachine` and `VirtualMachineBlockDeviceAttachments` phases", func() {
			By(fmt.Sprintf("`VirtualMachineBlockDeviceAttachments` should be in the %q phase", virtv2.BlockDeviceAttachmentPhaseAttached), func() {
				WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			By("`VirtualMachine` should be ready", func() {
				WaitVmReady(kc.WaitOptions{
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

		It("checks that `ISO` image is attached as `CD-ROM`", func() {
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
			Expect(isolockDeviceCount).To(BeNumerically("==", 1), "there is only one `ISO` block device in this case")
			isCdRom, err := IsBlockDeviceCdRom(vmObj.Name, isoBlockDeviceName)
			Expect(err).NotTo(HaveOccurred(), "failed to get `BlockDeviceType` of %q: %s", isoBlockDeviceName, err)
			Expect(isCdRom).Should(BeTrue(), "wrong type of block device: %s", isoBlockDeviceName)
		})

		It("check that images are attached as `ReadOnly` devices", func() {
			imgs := make(map[string]string, 0)

			cviImgs := &virtv2.ClusterVirtualImageList{}
			err := GetObjects(virtv2.ClusterVirtualImageResource, cviImgs, kc.GetOptions{
				Labels: testCaseLabel,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to get `ClusterVirtualImages`: %s", err)
			for _, cvi := range cviImgs.Items {
				diskName := fmt.Sprintf("cvi-%s", cvi.Name)
				imgs[diskName] = ""
			}

			viImgs := &virtv2.VirtualImageList{}
			err = GetObjects(virtv2.VirtualImageResource, viImgs, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to get `ClusterVirtualImages`: %s", err)
			for _, vi := range viImgs.Items {
				diskName := fmt.Sprintf("vi-%s", vi.Name)
				imgs[diskName] = ""
			}

			intVirtVmi := &virtv1.VirtualMachineInstance{}
			err = GetObject(kc.ResourceKubevirtVMI, vmObj.Name, intVirtVmi, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to get `InternalVirtulMachineInstance`: %s", err)
			for _, disk := range intVirtVmi.Spec.Domain.Devices.Disks {
				if _, ok := imgs[disk.Name]; ok {
					if strings.HasSuffix(disk.Name, isoLabel) {
						imgs[disk.Name] = fmt.Sprintf("%s-%s", CdRomIdPrefix, disk.Name)
					} else {
						imgs[disk.Name] = fmt.Sprintf("%s_%s", DiskIdPrefix, disk.Serial)
					}
				}
			}

			Expect(len(imgs)).To(Equal(imgCount), "there are only %d `blockDevices` in this case", imgCount)
			for img, diskId := range imgs {
				err := MountBlockDevice(vmObj.Name, diskId)
				Expect(err).NotTo(HaveOccurred(), "failed to mount %q into `VirtualMachine`: %s", img, err)
				isReadOnly, err := IsBlockDeviceReadOnly(vmObj.Name, diskId)
				Expect(err).NotTo(HaveOccurred(), "failed to check `ReadOnly` status: %s", img)
				Expect(isReadOnly).Should(BeTrue(), "mounted disk should be `ReadOnly`")
			}
		})
	})

	Context("When the virtual machine is ready", func() {
		It("detaches images", func() {
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
