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
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	. "github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const unacceptableCount = -1000

var APIVersion = virtv2.SchemeGroupVersion.String()

// lsblk JSON output
type Disks struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}

type BlockDevice struct {
	Name string `json:"name"`
	Size string `json:"size"`
}

func AttachVirtualDisk(virtualMachine, virtualDisk string, labels map[string]string) {
	vmbdaFilePath := fmt.Sprintf("%s/vmbda/%s.yaml", conf.TestData.VmDiskAttachment, virtualMachine)
	fmt.Println(vmbdaFilePath)
	err := CreateVMBDAManifest(vmbdaFilePath, virtualMachine, virtualDisk, labels)
	Expect(err).NotTo(HaveOccurred(), err)

	res := kubectl.Apply(kc.ApplyOptions{
		Filename:       []string{vmbdaFilePath},
		FilenameOption: kc.Filename,
		Namespace:      conf.Namespace,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
}

func CreateVMBDAManifest(filePath, vmName, vdName string, labels map[string]string) error {
	vmbda := &virtv2.VirtualMachineBlockDeviceAttachment{
		TypeMeta: v1.TypeMeta{
			APIVersion: APIVersion,
			Kind:       virtv2.VirtualMachineBlockDeviceAttachmentKind,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:   vmName,
			Labels: labels,
		},
		Spec: virtv2.VirtualMachineBlockDeviceAttachmentSpec{
			VirtualMachineName: vmName,
			BlockDeviceRef: virtv2.VMBDAObjectRef{
				Kind: virtv2.VMBDAObjectRefKindVirtualDisk,
				Name: vdName,
			},
		},
	}

	err := WriteYamlObject(filePath, vmbda)
	if err != nil {
		return err
	}

	return nil
}

func GetDisksMetadata(vmName string, disks *Disks) error {
	GinkgoHelper()
	cmd := "lsblk --nodeps --json"
	res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
		Namespace:   conf.Namespace,
		Username:    conf.TestData.SshUser,
		IdenityFile: conf.TestData.Sshkey,
	})
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}
	err := json.Unmarshal(res.StdOutBytes(), disks)
	if err != nil {
		return fmt.Errorf("failed when getting disk count\nvirtualMachine: %s/%s\nstderr: %s", conf.Namespace, vmName, res.StdErr())
	}
	return nil
}

var _ = Describe("Virtual disk attachment", ginkgoutil.CommonE2ETestDecorators(), func() {
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
		vmbdaName          string
	)

	Context("Preparing the environment", func() {
		vdAttach = fmt.Sprintf("%s-vd-attach-%s", namePrefix, nameSuffix)
		vmName = fmt.Sprintf("%s-vm-%s", namePrefix, nameSuffix)
		vmbdaName = fmt.Sprintf("%s-vm-%s", namePrefix, nameSuffix)

		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.VmDiskAttachment, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VmDiskAttachment},
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
				Namespace: conf.Namespace,
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
				Namespace:      conf.Namespace,
				Timeout:        MaxWaitTimeout,
			})
			By(fmt.Sprintf("VDs without consumers should be in %s phases", phaseByVolumeBindingMode))
			WaitPhaseByLabel(kc.ResourceVD, phaseByVolumeBindingMode, kc.WaitOptions{
				Labels:    hasNoConsumerLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("VMs should be running")
			WaitVmReady(true, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Describe("Attachment", func() {
		Context(fmt.Sprintf("When virtual machines are in %s phases", PhaseRunning), func() {
			It("get disk count before attachment", func() {
				Eventually(func() error {
					return GetDisksMetadata(vmName, &disksBefore)
				}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred(), "virtualMachine: %s", vmName)
			})
			It("attaches virtual disk", func() {
				AttachVirtualDisk(vmName, vdAttach, testCaseLabel)
			})
			It("checks VM and VMBDA phases", func() {
				By(fmt.Sprintf("VMBDA should be in %s phases", PhaseAttached))
				WaitPhaseByLabel(kc.ResourceVMBDA, PhaseAttached, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
				By("Virtual machines should be running")
				WaitVmReady(true, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			It("compares disk count before and after attachment", func() {
				diskCountBefore := len(disksBefore.BlockDevices)
				Eventually(func() (int, error) {
					err := GetDisksMetadata(vmName, &disksAfter)
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
					return GetDisksMetadata(vmName, &disksBefore)
				}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred(), "virtual machine: %s", vmName)
			})
			It("detaches virtual disk", func() {
				res := kubectl.Delete(kc.DeleteOptions{
					Filename:  []string{vmbdaName},
					Namespace: conf.Namespace,
					Resource:  kc.ResourceVMBDA,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
			})
			It("checks VM phase", func() {
				By("Virtual machines should be running")
				WaitVmReady(true, kc.WaitOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Timeout:   MaxWaitTimeout,
				})
			})
			It("compares disk count before and after detachment", func() {
				diskCountBefore := len(disksBefore.BlockDevices)
				Eventually(func() (int, error) {
					err := GetDisksMetadata(vmName, &disksAfter)
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
			DeleteTestCaseResources(ResourcesToDelete{
				KustomizationDir: conf.TestData.VmDiskAttachment,
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
