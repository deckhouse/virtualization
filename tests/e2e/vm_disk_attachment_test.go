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
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	. "github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const APIVersion = "virtualization.deckhouse.io/v1alpha2"

// lsblk JSON output
type Disks struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}

type BlockDevice struct {
	Name string `json:"name"`
	Size string `json:"size"`
}

var (
	DisksBefore                     Disks
	DisksAfter                      Disks
	AutomaticHotplugStandaloneLabel = map[string]string{"vm": "automatic-with-hotplug-standalone"}
)

func AttachVirtualDisk(virtualMachine, virtualDisk string) {
	vmbdaFilePath := fmt.Sprintf("%s/vmbda/%s.yaml", conf.TestData.VmDiskAttachment, virtualMachine)
	fmt.Println(vmbdaFilePath)
	err := CreateVMBDAManifest(vmbdaFilePath, virtualMachine, virtualDisk, AutomaticHotplugStandaloneLabel)
	Expect(err).NotTo(HaveOccurred(), err)

	res := kubectl.Apply(vmbdaFilePath, kc.ApplyOptions{
		Namespace: conf.Namespace,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
}

func CreateVMBDAManifest(filePath, vmName, vdName string, labels map[string]string) error {
	vmbda := &virtv2.VirtualMachineBlockDeviceAttachment{
		TypeMeta: v1.TypeMeta{
			APIVersion: APIVersion,
			Kind:       virtv2.VMBDAKind,
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

func GetDisksMetadata(vmName string, disks *Disks) {
	GinkgoHelper()
	cmd := "lsblk --nodeps --json"
	Eventually(func(g Gomega) {
		res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
			Namespace:   conf.Namespace,
			Username:    conf.TestData.SshUser,
			IdenityFile: conf.TestData.Sshkey,
		})
		g.Expect(res.Error()).NotTo(HaveOccurred(), "getting disk count failed for %s/%s.\n%s\n", conf.Namespace, vmName, res.StdErr())
		err := json.Unmarshal(res.StdOutBytes(), disks)
		g.Expect(err).NotTo(HaveOccurred())
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
}

var _ = Describe("Virtual disk attachment", Ordered, ContinueOnFailure, func() {
	var (
		vdRoot                   string
		vdAttach                 string
		vmName                   string
		vmbdaName                string
		phaseByVolumeBindingMode string
	)

	Context("Environment preparing", func() {
		vdRoot = fmt.Sprintf("%s-vd-root-%s", namePrefix, AutomaticHotplugStandaloneLabel["vm"])
		vdAttach = fmt.Sprintf("%s-vd-attach-%s", namePrefix, AutomaticHotplugStandaloneLabel["vm"])
		vmName = fmt.Sprintf("%s-vm-%s", namePrefix, AutomaticHotplugStandaloneLabel["vm"])
		vmbdaName = fmt.Sprintf("%s-vm-%s", namePrefix, AutomaticHotplugStandaloneLabel["vm"])
		switch conf.StorageClass.VolumeBindingMode {
		case "Immediate":
			phaseByVolumeBindingMode = PhaseReady
		case "WaitForFirstConsumer":
			phaseByVolumeBindingMode = PhaseWaitForFirstConsumer
		default:
			phaseByVolumeBindingMode = PhaseReady
		}
	})

	Context("When resources are applied:", func() {
		It("must have no errors", func() {
			res := kubectl.Kustomize(conf.TestData.VmDiskAttachment, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("%s should be in %s phases", vdRoot, PhaseReady))
			waitReady := fmt.Sprintf("'jsonpath={.status.phase}=%s'", PhaseReady)
			WaitResource(kc.ResourceVD, vdRoot, waitReady, LongWaitDuration)
			By(fmt.Sprintf("%s should be in %s phases", vdAttach, phaseByVolumeBindingMode))
			waitByVolumeBindingMode := fmt.Sprintf("'jsonpath={.status.phase}=%s'", phaseByVolumeBindingMode)
			WaitResource(kc.ResourceVD, vdAttach, waitByVolumeBindingMode, LongWaitDuration)
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    AutomaticHotplugStandaloneLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Describe("Attachment", func() {
		Context(fmt.Sprintf("When virtual machines are in %s phases:", PhaseRunning), func() {
			It("get disk count before attachment", func() {
				GetDisksMetadata(vmName, &DisksBefore)
			})
			It("attaches virtual disk", func() {
				AttachVirtualDisk(vmName, vdAttach)
			})
			It("checks VM and VMBDA phases", func() {
				By(fmt.Sprintf("VMBDA should be in %s phases", PhaseAttached))
				WaitResource(kc.ResourceVMBDA, vmbdaName, "'jsonpath={.status.phase}=Attached'", ShortWaitDuration)
				By(fmt.Sprintf("Virtual machines should be in %s phase", PhaseRunning))
				WaitResource(kc.ResourceVM, vmName, "'jsonpath={.status.phase}=Running'", ShortWaitDuration)
			})
			It("compares disk count before and after attachment", func() {
				GetDisksMetadata(vmName, &DisksAfter)
				diskCountBefore := len(DisksBefore.BlockDevices)
				diskCountAfter := len(DisksAfter.BlockDevices)
				Expect(diskCountBefore).To(Equal(diskCountAfter-1), "compare error: 'before' must be equal 'after - 1', before: %d, after: %d", diskCountBefore, diskCountAfter)
			})
		})
	})

	Describe("Detachment", func() {
		Context(fmt.Sprintf("When virtual machines are in %s phases:", PhaseRunning), func() {
			It("get disk count before detachment", func() {
				GetDisksMetadata(vmName, &DisksBefore)
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
					GetDisksMetadata(vmName, &DisksAfter)
					diskCountAfter := len(DisksAfter.BlockDevices)
					Expect(diskCountBefore).To(Equal(diskCountAfter+1), "compare error: 'before' must be equal 'after - 1', before: %d, after: %d", diskCountBefore, diskCountAfter)
				}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
			})
		})
	})
})
