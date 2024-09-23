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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	yamlv3 "gopkg.in/yaml.v3"

	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/deckhouse/virtualization/tests/e2e/resources"
)

type Disks struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}

type BlockDevice struct {
	Name string `json:"name"`
}

var (
	DisksBefore         Disks
	DisksAfter          Disks
	DiskAttachmentLabel = map[string]string{"testcase": "vm-disk-attachment"}
)

func AttachVirtualDisk(virtualMachine, virtualDisk string) {
	templatePath := fmt.Sprintf("%s/vmbda/vmbda.yaml", conf.TestData.VmDiskAttachment)
	template, err := GetManifest[VirtualMachineBlockDeviceAttachment](templatePath)
	Expect(err).NotTo(HaveOccurred(), err)

	vmbdaFilePath := fmt.Sprintf("%s/vmbda/%s.yaml", conf.TestData.VmDiskAttachment, virtualMachine)
	createErr := CreateVMBDAManifest(virtualMachine, virtualDisk, vmbdaFilePath, automaticHotplugStandaloneLabel, template)
	Expect(createErr).NotTo(HaveOccurred(), createErr)

	applyRes := kubectl.Apply(vmbdaFilePath, kc.ApplyOptions{
		Namespace: conf.Namespace,
	})
	Expect(applyRes.Error()).NotTo(HaveOccurred(), applyRes.StdErr())
}

func CreateVMBDAManifest(vmName, vdName, filePath string, labels map[string]string, template *VirtualMachineBlockDeviceAttachment) error {
	template.Metadata.Name = vmName
	template.Spec.VirtualMachineName = vmName
	template.Spec.BlockDeviceRef.Name = vdName
	template.Metadata.Labels = labels
	data, marshalErr := yamlv3.Marshal(template)
	if marshalErr != nil {
		return marshalErr
	}
	writeErr := os.WriteFile(filePath, data, 0o644)
	if writeErr != nil {
		return writeErr
	}
	return nil
}

func GetDiskCount(sshKeyPath, vmName string, disks *Disks) {
	GinkgoHelper()
	cmd := "lsblk --nodeps --noheadings --output name --json"
	Eventually(func(g Gomega) {
		res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
			Namespace:   conf.Namespace,
			Username:    "cloud",
			IdenityFile: sshKeyPath,
		})
		g.Expect(res.Error()).NotTo(HaveOccurred(), "getting disk count failed for %s/%s.\n%s\n%s", conf.Namespace, vmName, res.StdErr(), sshKeyPath)
		err := json.Unmarshal(res.StdOutBytes(), disks)
		g.Expect(err).NotTo(HaveOccurred())
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
}

var _ = Describe("Virtual disk attachment", Ordered, ContinueOnFailure, func() {
	Context("When resources are applied:", func() {
		It("must have no errors", func() {
			res := kubectl.Kustomize(conf.TestData.VmDiskAttachment, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				Labels:    DiskAttachmentLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    DiskAttachmentLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Describe("Attachment", func() {
		Context(fmt.Sprintf("When virtual machines are in %s phases:", PhaseRunning), func() {
			sshKeyPath := fmt.Sprintf("%s/id_ed", conf.TestData.Sshkeys)
			vmName := fmt.Sprintf("%s-vm-%s", namePrefix, automaticHotplugStandaloneLabel["vm"])
			vdName := fmt.Sprintf("%s-vd-attach-%s", namePrefix, automaticHotplugStandaloneLabel["vm"])
			It("get disk count before attachment", func() {
				GetDiskCount(sshKeyPath, vmName, &DisksBefore)
			})
			It("attaches virtual disk", func() {
				AttachVirtualDisk(vmName, vdName)
			})
			It("checks VM and VMBDA phases", func() {
				By(fmt.Sprintf("VMBDA should be in %s phases", PhaseAttached))
				vmbdaName := fmt.Sprintf("%s-vm-%s", namePrefix, automaticHotplugStandaloneLabel["vm"])
				WaitResource(kc.ResourceVMBDA, vmbdaName, "'jsonpath={.status.phase}=Attached'", ShortWaitDuration)
				By(fmt.Sprintf("Virtual machines should be in %s phase", PhaseRunning))
				WaitResource(kc.ResourceVM, vmName, "'jsonpath={.status.phase}=Running'", ShortWaitDuration)
			})
			It("compares disk count before and after attachment", func() {
				GetDiskCount(sshKeyPath, vmName, &DisksAfter)
				diskCountBefore := len(DisksBefore.BlockDevices)
				diskCountAfter := len(DisksAfter.BlockDevices)
				Expect(diskCountBefore).To(Equal(diskCountAfter-1), "compare error: 'before' must be equal 'after - 1', before: %d, after: %d", diskCountBefore, diskCountAfter)
			})
		})
	})

	Describe("Detachment", func() {
		Context(fmt.Sprintf("When virtual machines are in %s phases:", PhaseRunning), func() {
			sshKeyPath := fmt.Sprintf("%s/id_ed", conf.TestData.Sshkeys)
			vmName := fmt.Sprintf("%s-vm-%s", namePrefix, automaticHotplugStandaloneLabel["vm"])
			vmbdaName := fmt.Sprintf("%s-vm-%s", namePrefix, automaticHotplugStandaloneLabel["vm"])
			It("get disk count before detachment", func() {
				GetDiskCount(sshKeyPath, vmName, &DisksBefore)
			})
			It("detaches virtual disk", func() {
				res := kubectl.DeleteResource(kc.ResourceVMBDA, vmbdaName, kc.DeleteOptions{
					Namespace: conf.Namespace,
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
			})
			It("checks VM phase", func() {
				By(fmt.Sprintf("Virtual machines should be in %s phase", PhaseRunning))
				WaitResource(kc.ResourceVM, vmName, "'jsonpath={.status.phase}=Running'", ShortWaitDuration)
			})
			It("compares disk count before and after detachment", func() {
				diskCountBefore := len(DisksBefore.BlockDevices)
				Eventually(func(g Gomega) {
					GetDiskCount(sshKeyPath, vmName, &DisksAfter)
					diskCountAfter := len(DisksAfter.BlockDevices)
					Expect(diskCountBefore).To(Equal(diskCountAfter+1), "compare error: 'before' must be equal 'after - 1', before: %d, after: %d", diskCountBefore, diskCountAfter)
				}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
			})
		})
	})
})
