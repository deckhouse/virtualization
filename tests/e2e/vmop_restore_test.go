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
	virtualmachinerestoreoperationtest "github.com/deckhouse/virtualization/tests/e2e/virtual_machine_restore_operation_test"
)

var _ = Describe("VirtualMachineOperationRestore", Serial, ginkgoutil.CommonE2ETestDecorators(), func() {
	frameworkEntity := framework.NewFramework("virtual-machine-operation-restore")
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

		It("Creates file on the last disk", func() {
			Eventually(func(g Gomega) {
				helper.GeneratedValue = strconv.Itoa(time.Now().UTC().Second())

				res := d8Virtualization.SSHCommand(helper.VM.Name, helper.CreateFsAndSetValueOnDiskShell(helper.GeneratedValue), d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 10*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Creating snapshot", func() {
		It("Applying snapshot resource", func() {
			helper.VMSnapshot = vmsnapshotbuilder.New(
				vmsnapshotbuilder.WithName("vmsnapshot"),
				vmsnapshotbuilder.WithNamespace(frameworkEntity.Namespace().Name),
				vmsnapshotbuilder.WithVM(helper.VM.Name),
				vmsnapshotbuilder.WithRequiredConsistency(true),
				vmsnapshotbuilder.WithKeepIPAddress(v1alpha2.KeepIPAddressAlways),
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
				res := d8Virtualization.SSHCommand(helper.VM.Name, helper.ChangeValueOnDiskShell(helper.GetChangedValue()), d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 10*time.Second, time.Second).Should(Succeed())
		})

		It("Change VM spec", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				helper.VM.Annotations[helper.GetTestAnnotationName()] = helper.GetChangedValue()
				helper.VM.Labels[helper.GetTestLabelName()] = helper.GetChangedValue()
				helper.VM.Spec.CPU.Cores = 2
				helper.VM.Spec.Memory.Size = resource.MustParse("2Gi")

				err := helper.FrameworkEntity.Clients.GenericClient().Update(context.Background(), helper.VM)
				g.Expect(err).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("Reboot VM", func() {
			running, _ := conditions.GetCondition(vmcondition.TypeRunning, helper.VM.Status.Conditions)
			helper.RunningLastTransitionTime = running.LastTransitionTime.Time

			Eventually(func(g Gomega) {
				res := d8Virtualization.SSHCommand(helper.VM.Name, "sudo reboot", d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 10*time.Second, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				helper.UpdateState()

				running, _ := conditions.GetCondition(vmcondition.TypeRunning, helper.VM.Status.Conditions)
				g.Expect(running.LastTransitionTime.Time.After(helper.RunningLastTransitionTime)).Should(BeTrue())

				agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, helper.VM.Status.Conditions)
				g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
			}, 120*time.Second, time.Second).Should(Succeed())
		})

		It("VM spec should be changed", func() {
			Expect(helper.VM.Annotations[helper.GetTestAnnotationName()]).Should(Equal(helper.GetChangedValue()))
			Expect(helper.VM.Labels[helper.GetTestLabelName()]).Should(Equal(helper.GetChangedValue()))
			Expect(helper.VM.Spec.CPU.Cores).Should(Equal(2))
			Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("2Gi")))
		})
	})

	Context("Restore DryRun", func() {
		It("Applying DryRun restore VMOP", func() {
			helper.VMOPDryRun = vmopbuilder.New(
				vmopbuilder.WithName("vmop-dryrun"),
				vmopbuilder.WithNamespace(helper.FrameworkEntity.Namespace().Name),
				vmopbuilder.WithVirtualMachine(helper.VM.Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
				vmopbuilder.WithRestoreSpec(&v1alpha2.VirtualMachineOperationRestoreSpec{
					VirtualMachineSnapshotName: helper.VMSnapshot.Name,
					Mode:                       v1alpha2.VMOPRestoreModeDryRun,
				}),
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
			Expect(helper.VM.Annotations[helper.GetTestAnnotationName()]).Should(Equal(helper.GetChangedValue()))
			Expect(helper.VM.Labels[helper.GetTestLabelName()]).Should(Equal(helper.GetChangedValue()))
			Expect(helper.VM.Spec.CPU.Cores).Should(Equal(2))
			Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("2Gi")))
		})
	})

	Context("Restore BestEffort", func() {
		It("Applying BestEffort restore VMOP", func() {
			helper.VMOPBestEffort = vmopbuilder.New(
				vmopbuilder.WithName("vmop-best-effort"),
				vmopbuilder.WithNamespace(helper.FrameworkEntity.Namespace().Name),
				vmopbuilder.WithVirtualMachine(helper.VM.Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
				vmopbuilder.WithRestoreSpec(&v1alpha2.VirtualMachineOperationRestoreSpec{
					VirtualMachineSnapshotName: helper.VMSnapshot.Name,
					Mode:                       v1alpha2.VMOPRestoreModeBestEffort,
				}),
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
			helper.UpdateState()
			Expect(helper.VM.Annotations[helper.GetTestAnnotationName()]).Should(Equal(helper.GetDefaultValue()))
			Expect(helper.VM.Labels[helper.GetTestLabelName()]).Should(Equal(helper.GetDefaultValue()))
			Expect(helper.VM.Spec.CPU.Cores).Should(Equal(1))
			Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("1Gi")))
		})

		// It will be removed in the future: once the bug is fixed that causes a virtual machine to remain in the Stopped phase after recovery in BestEffort mode.
		It("Seems like a bug, wait and start VM", func() {
			time.Sleep(20 * time.Second)
			helper.UpdateState()

			if helper.VM.Status.Phase == v1alpha2.MachineStopped {
				By("Forcing VM start")

				startVMOP := vmopbuilder.New(
					vmopbuilder.WithName("start"),
					vmopbuilder.WithNamespace(helper.FrameworkEntity.Namespace().Name),
					vmopbuilder.WithVirtualMachine(helper.VM.Name),
					vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
				)
				err := helper.FrameworkEntity.Clients.GenericClient().Create(context.Background(), startVMOP)
				Expect(err).ShouldNot(HaveOccurred())
			}
		})

		It("Virtual Machine agent should be ready", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, helper.VM.Status.Conditions)
				g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("VMBDA should be attached", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				g.Expect(helper.VMBDA.Status.Phase).Should(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("File should have generated value", func() {
			Eventually(func(g Gomega) {
				res := d8Virtualization.SSHCommand(helper.VM.Name, helper.MountAndGetDiskFileContentShell(), d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())

				g.Expect(res.StdOut()).Should(ContainSubstring(helper.GeneratedValue))
			}, 10*time.Second, time.Second).Should(Succeed())
		})
	})

	Context("Changing VM", func() {
		It("Change data of created file", func() {
			Eventually(func(g Gomega) {
				res := d8Virtualization.SSHCommand(helper.VM.Name, helper.ChangeValueOnDiskShell("removed"), d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 10*time.Second, time.Second).Should(Succeed())
		})

		It("Change VM spec", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()

				helper.VM.Annotations[helper.GetTestAnnotationName()] = helper.GetChangedValue()
				helper.VM.Labels[helper.GetTestLabelName()] = helper.GetChangedValue()
				helper.VM.Spec.CPU.Cores = 2
				helper.VM.Spec.Memory.Size = resource.MustParse("2Gi")

				err := helper.FrameworkEntity.Clients.GenericClient().Update(context.Background(), helper.VM)
				g.Expect(err).ShouldNot(HaveOccurred())
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("Reboot VM", func() {
			running, _ := conditions.GetCondition(vmcondition.TypeRunning, helper.VM.Status.Conditions)
			helper.RunningLastTransitionTime = running.LastTransitionTime.Time

			Eventually(func(g Gomega) {
				res := d8Virtualization.SSHCommand(helper.VM.Name, "sudo reboot", d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())
			}, 10*time.Second, time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				helper.UpdateState()

				running, _ := conditions.GetCondition(vmcondition.TypeRunning, helper.VM.Status.Conditions)
				g.Expect(running.LastTransitionTime.Time.After(helper.RunningLastTransitionTime)).Should(BeTrue())

				agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, helper.VM.Status.Conditions)
				g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
			}, 120*time.Second, time.Second).Should(Succeed())
		})

		It("VM spec should be changed", func() {
			helper.UpdateState()
			Expect(helper.VM.Annotations[helper.GetTestAnnotationName()]).Should(Equal(helper.GetChangedValue()))
			Expect(helper.VM.Labels[helper.GetTestLabelName()]).Should(Equal(helper.GetChangedValue()))
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
			}, 10*time.Second, time.Second).Should(Succeed())
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
			helper.VMOPStrict = vmopbuilder.New(
				vmopbuilder.WithName("vmop-strict"),
				vmopbuilder.WithNamespace(helper.FrameworkEntity.Namespace().Name),
				vmopbuilder.WithVirtualMachine(helper.VM.Name),
				vmopbuilder.WithType(v1alpha2.VMOPTypeRestore),
				vmopbuilder.WithRestoreSpec(&v1alpha2.VirtualMachineOperationRestoreSpec{
					VirtualMachineSnapshotName: helper.VMSnapshot.Name,
					Mode:                       v1alpha2.VMOPRestoreModeStrict,
				}),
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

		// It will be removed in the future: we need to fix the bug that prevents VMBDA from being restored.
		It("Recreate VMBDA", func() {
			helper.VMBDA = vmbdabuilder.New(
				vmbdabuilder.WithName("vmbda"),
				vmbdabuilder.WithNamespace(helper.FrameworkEntity.Namespace().Name),
				vmbdabuilder.WithVMName(helper.VM.Name),
				vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: helper.VDBlank.Name,
				}),
			)
			err := frameworkEntity.Clients.GenericClient().Create(context.Background(), helper.VMBDA)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Waiting VMBDA", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				g.Expect(helper.VMBDA.Status.Phase).Should(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached))
			}, 120*time.Second, time.Second)
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
			helper.UpdateState()
			Expect(helper.VM.Annotations[helper.GetTestAnnotationName()]).Should(Equal(helper.GetDefaultValue()))
			Expect(helper.VM.Labels[helper.GetTestLabelName()]).Should(Equal(helper.GetDefaultValue()))
			Expect(helper.VM.Spec.CPU.Cores).Should(Equal(1))
			Expect(helper.VM.Spec.Memory.Size).Should(Equal(resource.MustParse("1Gi")))
		})

		It("Virtual Machine agent should be ready", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, helper.VM.Status.Conditions)
				g.Expect(agentReady.Status).Should(Equal(metav1.ConditionTrue))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("VMBDA should be attached", func() {
			Eventually(func(g Gomega) {
				helper.UpdateState()
				g.Expect(helper.VMBDA.Status.Phase).Should(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached))
			}, 60*time.Second, time.Second).Should(Succeed())
		})

		It("File should have generated value", func() {
			Eventually(func(g Gomega) {
				res := d8Virtualization.SSHCommand(helper.VM.Name, helper.MountAndGetDiskFileContentShell(), d8.SSHOptions{
					Namespace:   helper.VM.Namespace,
					Username:    conf.TestData.SSHUser,
					IdenityFile: conf.TestData.Sshkey,
				})
				g.Expect(res.Error()).ShouldNot(HaveOccurred())

				g.Expect(res.StdOut()).Should(ContainSubstring(helper.GeneratedValue))
			}, 10*time.Second, time.Second).Should(Succeed())
		})
	})
})
