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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const unacceptableCount = -1000

var APIVersion = virtv2.SchemeGroupVersion.String()

var _ = Describe("VirtualDisAttachment", ginkgoutil.CommonE2ETestDecorators(), func() {
	BeforeEach(func() {
		if config.IsReusable() {
			Skip("Test not available in REUSABLE mode: not supported yet.")
		}
	})

	var (
		testCaseLabel      = map[string]string{"testcase": "vm-disk-attachment"}
		hasNoConsumerLabel = map[string]string{"hasNoConsumer": "vm-disk-attachment"}
		nameSuffix         = "automatic-with-hotplug-standalone"
		disksBefore        Disks
		disksAfter         Disks
		vdAttach           string
		vmName             string
		ns                 string
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	Context("Preparing the environment", func() {
		vdAttach = fmt.Sprintf("%s-vd-attach-%s", namePrefix, nameSuffix)
		vmName = fmt.Sprintf("%s-vm-%s", namePrefix, nameSuffix)

		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMDiskAttachment, "kustomization.yaml")
			var err error
			ns, err = kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
		})
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VMDiskAttachment},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs with consumers should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				ExcludedLabels: []string{"hasNoConsumer"},
				Labels:         testCaseLabel,
				Namespace:      ns,
				Timeout:        MaxWaitTimeout,
			})
			By(fmt.Sprintf("VDs without consumers should be in %s phases", phaseByVolumeBindingMode))
			WaitPhaseByLabel(kc.ResourceVD, phaseByVolumeBindingMode, kc.WaitOptions{
				Labels:    hasNoConsumerLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Describe("Attachment", func() {
		Context("When virtual machine agents are ready", func() {
			It("get disk count before attachment", func() {
				Eventually(func() error {
					return GetDisksMetadata(ns, vmName, &disksBefore)
				}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred(), "virtualMachine: %s", vmName)
			})
			It("attaches virtual disk", func() {
				AttachBlockDevice(ns, vmName, vdAttach, virtv2.VMBDAObjectRefKindVirtualDisk, testCaseLabel, conf.TestData.VMDiskAttachment)
			})
			It("checks VM and VMBDA phases", func() {
				By(fmt.Sprintf("VMBDA should be in %s phases", PhaseAttached))
				WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machines should be ready")
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			It("compares disk count before and after attachment", func() {
				diskCountBefore := len(disksBefore.BlockDevices)
				Eventually(func() (int, error) {
					err := GetDisksMetadata(ns, vmName, &disksAfter)
					if err != nil {
						return unacceptableCount, err
					}
					diskCountAfter := len(disksAfter.BlockDevices)
					return diskCountAfter, nil
				}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(diskCountBefore+1), "comparing error: 'after' must be equal 'before + 1'")
			})
		})
	})

	Describe("Detachment", func() {
		Context(fmt.Sprintf("When virtual machines are in %s phases", PhaseRunning), func() {
			It("get disk count before detachment", func() {
				Eventually(func() error {
					return GetDisksMetadata(ns, vmName, &disksBefore)
				}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred(), "virtual machine: %s", vmName)
			})
			It("detaches virtual disk", func() {
				res := kubectl.Delete(kc.DeleteOptions{
					Filename:       []string{fmt.Sprintf("%s/vmbda", conf.TestData.VMDiskAttachment)},
					FilenameOption: kc.Filename,
					Namespace:      ns,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
			})
			It("checks VM phase", func() {
				By("Virtual machines should be ready")
				WaitVMAgentReady(kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: ns,
					Timeout:   MaxWaitTimeout,
				})
			})
			It("compares disk count before and after detachment", func() {
				diskCountBefore := len(disksBefore.BlockDevices)
				Eventually(func() (int, error) {
					err := GetDisksMetadata(ns, vmName, &disksAfter)
					if err != nil {
						return unacceptableCount, err
					}
					return len(disksAfter.BlockDevices), nil
				}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(diskCountBefore-1), "comparing error: 'after' must be equal 'before - 1'")
			})
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			DeleteTestCaseResources(ns, ResourcesToDelete{
				KustomizationDir: conf.TestData.VMDiskAttachment,
				AdditionalResources: []AdditionalResource{
					{
						Resource: kc.ResourceVMBDA,
						Labels:   testCaseLabel,
					},
				},
			})
		})
	})
})

// lsblk JSON output
type Disks struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}

type BlockDevices struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}

type BlockDevice struct {
	Name string `json:"name"`
	Size string `json:"size"`
	Type string `json:"type"`
}

func AttachBlockDevice(vmNamespace, vmName, blockDeviceName string, blockDeviceType virtv2.VMBDAObjectRefKind, labels map[string]string, testDataPath string) {
	vmbdaFilePath := fmt.Sprintf("%s/vmbda/%s.yaml", testDataPath, blockDeviceName)
	err := CreateVMBDAManifest(vmbdaFilePath, vmName, blockDeviceName, blockDeviceType, labels)
	Expect(err).NotTo(HaveOccurred(), "%v", err)

	res := kubectl.Apply(kc.ApplyOptions{
		Filename:       []string{vmbdaFilePath},
		FilenameOption: kc.Filename,
		Namespace:      vmNamespace,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
}

func CreateVMBDAManifest(filePath, vmName, blockDeviceName string, blockDeviceType virtv2.VMBDAObjectRefKind, labels map[string]string) error {
	vmbda := &virtv2.VirtualMachineBlockDeviceAttachment{
		TypeMeta: v1.TypeMeta{
			APIVersion: APIVersion,
			Kind:       virtv2.VirtualMachineBlockDeviceAttachmentKind,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:   blockDeviceName,
			Labels: labels,
		},
		Spec: virtv2.VirtualMachineBlockDeviceAttachmentSpec{
			VirtualMachineName: vmName,
			BlockDeviceRef: virtv2.VMBDAObjectRef{
				Kind: blockDeviceType,
				Name: blockDeviceName,
			},
		},
	}

	err := helper.WriteYamlObject(filePath, vmbda)
	if err != nil {
		return err
	}

	return nil
}

func GetDisksMetadata(vmNamespace, vmName string, disks *Disks) error {
	GinkgoHelper()
	cmd := "lsblk --nodeps --json"
	res := d8Virtualization.SSHCommand(vmName, cmd, d8.SSHOptions{
		Namespace:   vmNamespace,
		Username:    conf.TestData.SSHUser,
		IdenityFile: conf.TestData.Sshkey,
	})
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}
	err := json.Unmarshal(res.StdOutBytes(), disks)
	if err != nil {
		return fmt.Errorf("failed when getting disk count\nvirtualMachine: %s/%s\nstderr: %s", vmNamespace, vmName, res.StdErr())
	}
	return nil
}
