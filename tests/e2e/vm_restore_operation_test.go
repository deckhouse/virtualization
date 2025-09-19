/*
Copyright 2025 Flant JSC

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
	"context"
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
)

const ubuntuUrl = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/ubuntu/jammy-minimal-cloudimg-amd64.img"
const viUrl = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.qcow2"
const cviUrl = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/test/test.iso"

var _ = Describe("VirtualMachineRestoreOperation", Serial, ginkgoutil.CommonE2ETestDecorators(), func() {
	frameworkEntity := framework.NewFramework("virtual-machine-restore-operation")
	helper := NewVMOPRestoreTestHelper(frameworkEntity)

	frameworkEntity.BeforeAll()
	frameworkEntity.AfterAll()

	Context("Preparing resources", func() {
		It("Applying resources", func() {
			helper.GenerateAndCreateOriginalResources()
		})

		It("Resources should be ready", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				helper.CheckIfResourcesReady(g)
			}, 600*time.Second, 1*time.Second).Should(Succeed())
		})

		It("Creating file on vmbda", func() {
			Eventually(func(g Gomega) {
				helper.MagicValue = strconv.Itoa(time.Now().UTC().Second())
				cmdCreate := fmt.Sprintf("DEV=/dev/$(sudo lsblk | grep 52M | awk \"{print \\$1}\") && sudo mkfs.ext4 $DEV && sudo mount $DEV /mnt && sudo bash -c \"echo %s > /mnt/value\"", helper.MagicValue)

				res := d8Virtualization.SSHCommand(helper.VM.Name, cmdCreate, d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Creating snapshot", func() {
		It("Applying snapshot resource", func() {
			helper.VMSnapshot = helper.GenerateVMSnapshot(
				"vmsnapshot",
				frameworkEntity.Namespace().Name,
				helper.VM.Name,
				true,
				v1alpha2.KeepIPAddressAlways,
			)
			By(fmt.Sprintf("Creating vm snapshot: %s/%s", helper.VMSnapshot.Namespace, helper.VMSnapshot.Name))
			err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMSnapshot)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Snapshot should be Ready", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				g.Expect(helper.VMSnapshot.Status.Phase).Should(Equal(v1alpha2.VirtualMachineSnapshotPhaseReady))
			}, 60*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Changing VM", func() {
		It("Change data of created file", func() {
			Eventually(func(g Gomega) {
				cmd := "sudo bash -c \"echo removed > /mnt/value\""

				res := d8Virtualization.SSHCommand(helper.VM.Name, cmd, d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("Change VM spec", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				helper.VM.Annotations["test-label"] = "changed"
				helper.VM.Labels["test-label"] = "changed"
				helper.VM.Spec.CPU.Cores = 2
				helper.VM.Spec.Memory.Size = resource.MustParse("2Gi")

				err := helper.FrameworkEntity.Clients.GenericClient().Update(context.Background(), helper.VM)
				g.Expect(err).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("Reboot VM", func() {
			Eventually(func(g Gomega) {
				res := d8Virtualization.SSHCommand(helper.VM.Name, "sudo reboot", d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				helper.UpdateState()
				g.Expect(helper.VM.Status.Phase).Should(Equal(v1alpha2.MachineStopped))
			}, 60*time.Second, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				helper.UpdateState()
				agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, helper.VM.Status.Conditions)
				g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("VM spec should be changed", func() {
			Expect(helper.VM.Annotations["test-label"]).Should(Equal("changed"))
			Expect(helper.VM.Labels["test-label"]).Should(Equal("changed"))
			Expect(helper.VM.Spec.CPU.Cores).Should(Equal(2))
			Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("2Gi")))
		})
	})

	Context("Restore DryRun", func() {
		It("Applying DryRun restore VMOP", func() {
			helper.VMOPDryRun = helper.GenerateRestoreVMOP(
				"vmop-dryrun", frameworkEntity.Namespace().Name,
				helper.VMSnapshot.Name,
				helper.VM.Name,
				v1alpha2.VMOPRestoreModeDryRun,
			)
			err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMOPDryRun)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("VMOP should be Completed", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				g.Expect(helper.VMOPDryRun.Status.Phase).Should(Equal(v1alpha2.VMOPPhaseCompleted))

				restoreCompleted, _ := conditions.GetCondition(vmopcondition.TypeRestoreCompleted, helper.VMOPDryRun.Status.Conditions)
				g.Expect(restoreCompleted.Status).Should(Equal(metav1.ConditionTrue))
				g.Expect(restoreCompleted.Reason).Should(Equal(vmopcondition.ReasonDryRunOperationCompleted.String()))
				g.Expect(restoreCompleted.Message).Should(Equal("The virtual machine can be restored from the snapshot."))
			}, 120*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Check VM state", func() {
		It("VM should have changed state", func() {
			helper.UpdateState()
			Expect(helper.VM.Annotations["test-label"]).Should(Equal("changed"))
			Expect(helper.VM.Labels["test-label"]).Should(Equal("changed"))
			Expect(helper.VM.Spec.CPU.Cores).Should(Equal(2))
			Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("2Gi")))
		})
	})

	Context("Restore BestEffort", func() {
		It("Applying BestEffort restore VMOP", func() {
			helper.VMOPBestEffort = helper.GenerateRestoreVMOP(
				"vmop-best-effort", frameworkEntity.Namespace().Name,
				helper.VMSnapshot.Name,
				helper.VM.Name,
				v1alpha2.VMOPRestoreModeBestEffort,
			)
			err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMOPBestEffort)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("VMOP should be Completed", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				g.Expect(helper.VMOPBestEffort.Status.Phase).Should(Equal(v1alpha2.VMOPPhaseCompleted))
			}, 120*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Check VM state", func() {
		It("VM should have restored state", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				g.Expect(helper.VM.Annotations["test-annotation"]).Should(Equal("value"))
				g.Expect(helper.VM.Labels["test-label"]).Should(Equal("value"))
				g.Expect(helper.VM.Spec.CPU.Cores).Should(Equal(1))
				g.Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("1Gi")))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("Seems like a bug, wait and start VM", func() {
			time.Sleep(20 * time.Second)
			helper.UpdateState()

			if helper.VM.Status.Phase == v1alpha2.MachineStopped {
				By("Forcing VM start")

				startVMOP := helper.GenerateStartVMOP("start", helper.FrameworkEntity.Namespace().Name, helper.VM.Name)
				err := helper.FrameworkEntity.Clients.GenericClient().Create(context.Background(), startVMOP)
				Expect(err).ShouldNot(HaveOccurred())

				Eventually(func(g Gomega) {
					helper.UpdateState()
					agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, helper.VM.Status.Conditions)
					g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
				}, 60*time.Second, time.Second).Should(Succeed())
			}
		})

		It("File should have magic value", func() {
			Eventually(func(g Gomega) {
				cmdGet := "DEV=/dev/$(sudo lsblk | grep 52M | awk \"{print \\$1}\") && sudo mount $DEV /mnt && cat /mnt/value"

				res := d8Virtualization.SSHCommand(helper.VM.Name, cmdGet, d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())

				g.Expect(res.StdOut()).Should(ContainSubstring(helper.MagicValue))
			}, 60*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Changing VM", func() {
		It("Change data of created file", func() {
			Eventually(func(g Gomega) {
				cmd := "sudo bash -c \"echo removed > /mnt/value\""

				res := d8Virtualization.SSHCommand(helper.VM.Name, cmd, d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("Change VM spec", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				helper.VM.Annotations["test-label"] = "changed"
				helper.VM.Labels["test-label"] = "changed"
				helper.VM.Spec.CPU.Cores = 2
				helper.VM.Spec.Memory.Size = resource.MustParse("2Gi")

				err := helper.FrameworkEntity.Clients.GenericClient().Update(context.Background(), helper.VM)
				g.Expect(err).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("Reboot VM", func() {
			Eventually(func(g Gomega) {
				res := d8Virtualization.SSHCommand(helper.VM.Name, "sudo reboot", d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				helper.UpdateState()
				g.Expect(helper.VM.Status.Phase).Should(Equal(v1alpha2.MachineStopped))
			}, 60*time.Second, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				helper.UpdateState()
				agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, helper.VM.Status.Conditions)
				g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("VM spec should be changed", func() {
			helper.UpdateState()
			Expect(helper.VM.Annotations["test-label"]).Should(Equal("changed"))
			Expect(helper.VM.Labels["test-label"]).Should(Equal("changed"))
			Expect(helper.VM.Spec.CPU.Cores).Should(Equal(2))
			Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("2Gi")))
		})

		It("Shutdown VM", func() {
			Eventually(func(g Gomega) {
				d8Virtualization.SSHCommand(helper.VM.Name, "sudo poweroff", d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})

				helper.UpdateState()
				g.Expect(helper.VM.Status.Phase).Should(Equal(v1alpha2.MachineStopped))
			}, 60*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Removing resources", func() {
		It("Delete resources", func() {
			err := helper.FrameworkEntity.Clients.GenericClient().Delete(context.Background(), helper.VMBDA)
			Expect(err).ShouldNot(HaveOccurred())
			err = helper.FrameworkEntity.Clients.GenericClient().Delete(context.Background(), helper.VDBlank)
			Expect(err).ShouldNot(HaveOccurred())
			err = helper.FrameworkEntity.Clients.GenericClient().Delete(context.Background(), helper.VDRoot)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Resources should be deleted", func() {
			Eventually(func(g Gomega) {
				var vmbda v1alpha2.VirtualMachineBlockDeviceAttachment
				err := helper.FrameworkEntity.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
					Namespace: helper.VMBDA.Namespace,
					Name:      helper.VMBDA.Name,
				}, &vmbda)
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

				var vdRoot v1alpha2.VirtualDisk
				err = helper.FrameworkEntity.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
					Namespace: helper.VDRoot.Namespace,
					Name:      helper.VDRoot.Name,
				}, &vdRoot)
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())

				var vdBlank v1alpha2.VirtualDisk
				err = helper.FrameworkEntity.Clients.GenericClient().Get(context.Background(), types.NamespacedName{
					Namespace: helper.VDBlank.Namespace,
					Name:      helper.VDBlank.Name,
				}, &vdBlank)
				g.Expect(k8serrors.IsNotFound(err)).Should(BeTrue())
			}, 300*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Restore Strict", func() {
		It("Applying Strict restore VMOP", func() {
			helper.VMOPStrict = helper.GenerateRestoreVMOP(
				"vmop-strict", frameworkEntity.Namespace().Name,
				helper.VMSnapshot.Name,
				helper.VM.Name,
				v1alpha2.VMOPRestoreModeStrict,
			)
			err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMOPStrict)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("VMOP should be Completed", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				g.Expect(helper.VMOPStrict.Status.Phase).Should(Equal(v1alpha2.VMOPPhaseCompleted))
			}, 120*time.Second, 1*time.Second).Should(Succeed())
		})
	})

	Context("Check VM state", func() {
		It("VM agent should be ready", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, helper.VM.Status.Conditions)
				g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("VM should have restored state", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				g.Expect(helper.VM.Annotations["test-annotation"]).Should(Equal("value"))
				g.Expect(helper.VM.Labels["test-label"]).Should(Equal("value"))
				g.Expect(helper.VM.Spec.CPU.Cores).Should(Equal(1))
				g.Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("1Gi")))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("File should have magic value", func() {
			Eventually(func(g Gomega) {
				cmdGet := "DEV=/dev/$(sudo lsblk | grep 52M | awk \"{print \\$1}\") && sudo mount $DEV /mnt && cat /mnt/value"

				res := d8Virtualization.SSHCommand(helper.VM.Name, cmdGet, d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())

				g.Expect(res.StdOut()).Should(ContainSubstring(helper.MagicValue))
			}, 60*time.Second, time.Second).Should(Succeed())
		})
	})
})

type VMOPRestoreTestHelper struct {
	FrameworkEntity *framework.Framework
	VM              *v1alpha2.VirtualMachine
	CVI             *v1alpha2.ClusterVirtualImage
	VI              *v1alpha2.VirtualImage
	VDRoot, VDBlank *v1alpha2.VirtualDisk
	VMBDA           *v1alpha2.VirtualMachineBlockDeviceAttachment
	VMSnapshot      *v1alpha2.VirtualMachineSnapshot
	VMOPDryRun      *v1alpha2.VirtualMachineOperation
	VMOPStrict      *v1alpha2.VirtualMachineOperation
	VMOPBestEffort  *v1alpha2.VirtualMachineOperation
	MagicValue      string
}

func NewVMOPRestoreTestHelper(frameworkEntity *framework.Framework) *VMOPRestoreTestHelper {
	return &VMOPRestoreTestHelper{
		FrameworkEntity: frameworkEntity,
	}
}

func (h *VMOPRestoreTestHelper) GenerateAndCreateOriginalResources() {
	GinkgoHelper()
	h.CVI = h.GenerateCVI("ubuntu-cvi", cviUrl)

	// for getting real cvi name
	err := h.FrameworkEntity.GenericClient().Create(context.Background(), h.CVI)
	By(fmt.Sprintf("Created cvi: %s", h.CVI.Name))
	Expect(err).ShouldNot(HaveOccurred())

	h.FrameworkEntity.AddResourceToDelete(h.CVI)
	h.VI = h.GenerateVI("ubuntu-vi", h.FrameworkEntity.Namespace().Name, viUrl)
	h.VDRoot = h.GenerateVDFromHttp("vd-root", h.FrameworkEntity.Namespace().Name, "10Gi", ubuntuUrl)
	h.VDBlank = h.GenerateVDBlank("vd-blank", h.FrameworkEntity.Namespace().Name, "51Mi")
	h.VM = h.GenerateVM(
		"ubuntu-vm",
		h.FrameworkEntity.Namespace().Name,
		v1alpha2.AlwaysSafeMigrationPolicy,
		1,
		"10%",
		"1Gi",
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: h.VDRoot.Name,
		},
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ClusterImageDevice,
			Name: h.CVI.Name,
		},
		v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.ImageDevice,
			Name: h.VI.Name,
		},
	)
	h.VMBDA = h.GenerateVMBDA(
		"vmbda", h.FrameworkEntity.Namespace().Name, h.VM.Name,
		v1alpha2.VMBDAObjectRef{
			Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
			Name: h.VDBlank.Name,
		},
	)

	By(fmt.Sprintf("Creating vi: %s/%s", h.VI.Namespace, h.VI.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VI)
	Expect(err).ShouldNot(HaveOccurred())
	By(fmt.Sprintf("Creating vd blank: %s/%s", h.VDBlank.Namespace, h.VDBlank.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VDBlank)
	Expect(err).ShouldNot(HaveOccurred())
	By(fmt.Sprintf("Creating vd root: %s/%s", h.VDRoot.Namespace, h.VDRoot.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VDRoot)
	Expect(err).ShouldNot(HaveOccurred())
	By(fmt.Sprintf("Creating vm: %s/%s", h.VM.Namespace, h.VM.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VM)
	Expect(err).ShouldNot(HaveOccurred())
	By(fmt.Sprintf("Creating vmbda: %s/%s", h.VMBDA.Namespace, h.VMBDA.Name))
	err = h.FrameworkEntity.GenericClient().Create(context.Background(), h.VMBDA)
	Expect(err).ShouldNot(HaveOccurred())
}

func (h *VMOPRestoreTestHelper) GenerateVM(
	name, namespace string,
	liveMigrationPolicy v1alpha2.LiveMigrationPolicy,
	cores int,
	coreFraction string,
	memorySize string,
	blockDeviceRefs ...v1alpha2.BlockDeviceSpecRef,
) *v1alpha2.VirtualMachine {
	cloudInit :=
		`#cloud-config
users:
- name: cloud
  passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
  shell: /bin/bash
  sudo: ALL=(ALL) NOPASSWD:ALL
  chpasswd: { expire: False }
  lock_passwd: false
  ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com

runcmd:
- [bash, -c, "apt update"]
- [bash, -c, "apt install qemu-guest-agent -y"]
- [bash, -c, "systemctl enable qemu-guest-agent"]
- [bash, -c, "systemctl start qemu-guest-agent"]`

	return vmbuilder.New(
		vmbuilder.WithAnnotation("test-annotation", "value"),
		vmbuilder.WithLabel("test-label", "value"),
		vmbuilder.WithName(name),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithBlockDeviceRefs(blockDeviceRefs...),
		vmbuilder.WithLiveMigrationPolicy(liveMigrationPolicy),
		vmbuilder.WithCPU(cores, ptr.To(coreFraction)),
		vmbuilder.WithMemory(resource.MustParse(memorySize)),
		vmbuilder.WithProvisioning(
			&v1alpha2.Provisioning{
				Type:     v1alpha2.ProvisioningTypeUserData,
				UserData: cloudInit,
			},
		),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVDBlank(name, namespace, size string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVDFromHttp(name, namespace, size, url string) *v1alpha2.VirtualDisk {
	return vdbuilder.New(
		vdbuilder.WithName(name),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithSize(ptr.To(resource.MustParse(size))),
		vdbuilder.WithDataSourceHTTPWithOnlyURL(url),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVI(name, namespace, url string) *v1alpha2.VirtualImage {
	return vibuilder.New(
		vibuilder.WithName(name),
		vibuilder.WithNamespace(namespace),
		vibuilder.WithDataSourceHTTPWithOnlyURL(url),
		vibuilder.WithStorageType(ptr.To(v1alpha2.StoragePersistentVolumeClaim)),
	)
}

func (h *VMOPRestoreTestHelper) GenerateCVI(name, url string) *v1alpha2.ClusterVirtualImage {
	return cvibuilder.New(
		cvibuilder.WithGenerateName(fmt.Sprintf("%s-", name)),
		cvibuilder.WithDataSourceHTTPWithOnlyURL(url),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVMBDA(name, namespace, vmName string, bdRef v1alpha2.VMBDAObjectRef) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return vmbdabuilder.New(
		vmbdabuilder.WithName(name),
		vmbdabuilder.WithNamespace(namespace),
		vmbdabuilder.WithVMName(vmName),
		vmbdabuilder.WithBlockDeviceRef(bdRef),
	)
}

func (h *VMOPRestoreTestHelper) GenerateVMSnapshot(
	name, namespace, vmName string,
	requiredConsistency bool,
	keepIpAddress v1alpha2.KeepIPAddress,
) *v1alpha2.VirtualMachineSnapshot {
	return vmsnapshotbuilder.New(
		vmsnapshotbuilder.WithName(name),
		vmsnapshotbuilder.WithNamespace(namespace),
		vmsnapshotbuilder.WithVm(vmName),
		vmsnapshotbuilder.WithKeepIpAddress(keepIpAddress),
		vmsnapshotbuilder.WithRequiredConsistency(requiredConsistency),
	)
}

func (h *VMOPRestoreTestHelper) GenerateRestoreVMOP(name, namespace, vmSnapshotName, vmName string, restoreMode v1alpha2.VMOPRestoreMode) *v1alpha2.VirtualMachineOperation {
	restoreSpec := &v1alpha2.VirtualMachineOperationRestoreSpec{
		VirtualMachineSnapshotName: vmSnapshotName,
		Mode:                       restoreMode,
	}

	return vmopbuilder.New(
		vmopbuilder.WithName(name),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
		vmopbuilder.WithRestoreSpec(restoreSpec),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func (h *VMOPRestoreTestHelper) GenerateRestartVMOP(name, namespace, vmName string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithName(name),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestart),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func (h *VMOPRestoreTestHelper) GenerateStartVMOP(name, namespace, vmName string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithName(name),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func (h *VMOPRestoreTestHelper) UpdateState() {
	var err error

	if h.CVI != nil {
		var cvi v1alpha2.ClusterVirtualImage
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Name: h.CVI.Name,
			},
			&cvi,
		)
		if err == nil {
			h.CVI = &cvi
		}
	}

	if h.VI != nil {
		var vi v1alpha2.VirtualImage
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VI.Namespace,
				Name:      h.VI.Name,
			},
			&vi,
		)
		if err == nil {
			h.VI = &vi
		}
	}

	if h.VDBlank != nil {
		var vdBlank v1alpha2.VirtualDisk
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VDBlank.Namespace,
				Name:      h.VDBlank.Name,
			},
			&vdBlank,
		)
		if err == nil {
			h.VDBlank = &vdBlank
		}
	}

	if h.VDRoot != nil {
		var vdRoot v1alpha2.VirtualDisk
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VDRoot.Namespace,
				Name:      h.VDRoot.Name,
			},
			&vdRoot,
		)
		if err == nil {
			h.VDRoot = &vdRoot
		}
	}

	if h.VMBDA != nil {
		var vmbda v1alpha2.VirtualMachineBlockDeviceAttachment
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMBDA.Namespace,
				Name:      h.VMBDA.Name,
			},
			&vmbda,
		)
		if err == nil {
			h.VMBDA = &vmbda
		}
	}

	if h.VM != nil {
		var vm v1alpha2.VirtualMachine
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VM.Namespace,
				Name:      h.VM.Name,
			},
			&vm,
		)
		if err == nil {
			h.VM = &vm
		}
	}

	if h.VMSnapshot != nil {
		var vmSnapshot v1alpha2.VirtualMachineSnapshot
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMSnapshot.Namespace,
				Name:      h.VMSnapshot.Name,
			},
			&vmSnapshot,
		)
		if err == nil {
			h.VMSnapshot = &vmSnapshot
		}
	}

	if h.VMOPDryRun != nil {
		var vmopDryRun v1alpha2.VirtualMachineOperation
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMOPDryRun.Namespace,
				Name:      h.VMOPDryRun.Name,
			},
			&vmopDryRun,
		)
		if err == nil {
			h.VMOPDryRun = &vmopDryRun
		}
	}

	if h.VMOPStrict != nil {
		var vmopStrict v1alpha2.VirtualMachineOperation
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMOPStrict.Namespace,
				Name:      h.VMOPStrict.Name,
			},
			&vmopStrict,
		)
		if err == nil {
			h.VMOPStrict = &vmopStrict
		}
	}

	if h.VMOPBestEffort != nil {
		var vmopBestEffort v1alpha2.VirtualMachineOperation
		err = h.FrameworkEntity.Clients.GenericClient().Get(
			context.Background(),
			types.NamespacedName{
				Namespace: h.VMOPBestEffort.Namespace,
				Name:      h.VMOPBestEffort.Name,
			},
			&vmopBestEffort,
		)
		if err == nil {
			h.VMOPBestEffort = &vmopBestEffort
		}
	}
}

func (h *VMOPRestoreTestHelper) CheckIfResourcesReady(g Gomega) {
	g.Expect(h.CVI.Status.Phase).Should(Equal(v1alpha2.ImageReady))
	g.Expect(h.VI.Status.Phase).Should(Equal(v1alpha2.ImageReady))
	g.Expect(h.VDBlank.Status.Phase).Should(Equal(v1alpha2.DiskReady))
	g.Expect(h.VDRoot.Status.Phase).Should(Equal(v1alpha2.DiskReady))
	g.Expect(h.VMBDA.Status.Phase).Should(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached))
	g.Expect(h.VM.Status.Phase).Should(Equal(v1alpha2.MachineRunning))

	agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, h.VM.Status.Conditions)
	g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
}
