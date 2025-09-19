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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	virtualmachinerestoreoperationtest "github.com/deckhouse/virtualization/tests/e2e/virtual_machine_restore_operation_test"
	"github.com/deckhouse/virtualization/tests/e2e/virtual_machine_restore_operation_test/resources"
)

var _ = Describe("VirtualMachineRestoreOperation", Serial, ginkgoutil.CommonE2ETestDecorators(), func() {
	frameworkEntity := framework.NewFramework("virtual-machine-restore-operation")
	helper := virtualmachinerestoreoperationtest.NewVMOPRestoreTestHelper(frameworkEntity)

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
			helper.VMSnapshot = resources.NewVMSnapshot(
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
			helper.VMOPDryRun = resources.NewRestoreDryRunVMOP(
				"vmop-dryrun", frameworkEntity.Namespace().Name,
				helper.VMSnapshot.Name,
				helper.VM.Name,
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
			helper.VMOPBestEffort = resources.NewRestoreBestEffortVMOP(
				"vmop-best-effort", frameworkEntity.Namespace().Name,
				helper.VMSnapshot.Name,
				helper.VM.Name,
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

				startVMOP := resources.NewStartVMOP("start", helper.FrameworkEntity.Namespace().Name, helper.VM.Name)
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
			helper.VMOPStrict = resources.NewRestoreStrictVMOP(
				"vmop-strict", frameworkEntity.Namespace().Name,
				helper.VMSnapshot.Name,
				helper.VM.Name,
			)
			err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMOPStrict)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("VMOP should be Completed", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				g.Expect(helper.VMOPStrict.Status.Phase).Should(Equal(v1alpha2.VMOPPhaseCompleted))
			}, 60*time.Second, 1*time.Second).Should(Succeed())
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
