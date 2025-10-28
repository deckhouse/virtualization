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

package vm

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("LocalVirtualDiskMigration", Ordered, func() {
	var (
		f            = framework.NewFramework("volume-migration-local-disks")
		storageClass *storagev1.StorageClass
		vi           *v1alpha2.VirtualImage
	)

	BeforeEach(func() {
		// TODO: Remove Skip after fixing the issue.
		Skip("This test case is not working everytime. Should be fixed.")

		storageClass = framework.GetConfig().StorageClass.TemplateStorageClass
		if storageClass == nil {
			Skip("TemplateStorageClass is not set.")
		}

		f.Before()

		DeferCleanup(f.After)

		newVI := object.NewGeneratedHTTPVIUbuntu("volume-migration-local-disks-")
		newVI, err := f.VirtClient().VirtualImages(f.Namespace().Name).Create(context.Background(), newVI, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(newVI)
		vi = newVI
	})

	const (
		vdRootName       = "vd-ubuntu-root-disk"
		vdAdditionalName = "vd-ubuntu-additional-disk"
	)

	localMigrationRootOnlyBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return onlyRootBuild(f, vi, buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: true})
	}

	localMigrationRootAndAdditionalBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return rootAndAdditionalBuild(f, vi,
			buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: true},
			buildOption{name: vdAdditionalName, storageClass: &storageClass.Name, rwo: true},
		)
	}

	localMigrationAdditionalOnlyBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return onlyAdditionalBuild(f, vi,
			buildOption{name: vdRootName, rwo: false},
			buildOption{name: vdAdditionalName, rwo: true},
		)
	}

	DescribeTable("should be successful", func(build func() (vm *v1alpha2.VirtualMachine, vds []*v1alpha2.VirtualDisk)) {
		ns := f.Namespace().Name

		vm, vds := build()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		const vmopName = "local-disks-migration"

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(vm, vmopbuilder.WithName(vmopName))

		Eventually(func(g Gomega) {
			vmop, err := f.VirtClient().VirtualMachineOperations(ns).Get(context.Background(), vmopName, metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())

		vm, err = f.VirtClient().VirtualMachines(ns).Get(context.Background(), vm.GetName(), metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.Status.MigrationState).ShouldNot(BeNil())
		Expect(vm.Status.MigrationState.EndTimestamp).ShouldNot(BeNil())
		Expect(vm.Status.MigrationState.Result).To(Equal(v1alpha2.MigrationResultSucceeded))

		untilVirtualDisksMigrationsSucceeded(f)
	},
		Entry("when only root disk on local storage", localMigrationRootOnlyBuild),
		Entry("when root disk on local storage and one additional disk", localMigrationRootAndAdditionalBuild),
		Entry("when only additional disk on local storage", localMigrationAdditionalOnlyBuild),
	)

	DescribeTable("should be reverted", func(build func() (vm *v1alpha2.VirtualMachine, vds []*v1alpha2.VirtualDisk)) {
		ns := f.Namespace().Name

		vm, vds := build()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		ExecStressNGInVirtualMachine(f, vm)

		const vmopName = "local-disks-migration"

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(vm, vmopbuilder.WithName(vmopName))

		untilVirtualMachinesWillBeStartMigratingAndCancelImmediately(f)

		untilVirtualDisksMigrationsFailed(f)
	},
		Entry("when only root disk on local storage", localMigrationRootOnlyBuild),
		Entry("when root disk on local storage and one additional disk", localMigrationRootAndAdditionalBuild),
		Entry("when only additional disk on local storage", localMigrationAdditionalOnlyBuild),
	)

	It("should be successful two migrations in a row", func() {
		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		for i := range 2 {
			vmopName := "local-disks-migration-" + strconv.Itoa(i)

			By("Starting migrations for virtual machines")
			util.MigrateVirtualMachine(vm, vmopbuilder.WithName(vmopName))

			Eventually(func(g Gomega) {
				vmop, err := f.VirtClient().VirtualMachineOperations(ns).Get(context.Background(), vmopName, metav1.GetOptions{})
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
			}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())

			vm, err = f.VirtClient().VirtualMachines(ns).Get(context.Background(), vm.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Status.MigrationState).ShouldNot(BeNil())
			Expect(vm.Status.MigrationState.EndTimestamp).ShouldNot(BeNil())
			Expect(vm.Status.MigrationState.Result).To(Equal(v1alpha2.MigrationResultSucceeded))

			untilVirtualDisksMigrationsSucceeded(f)
		}
	})

	It("should be reverted first and completed second", func() {
		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		ExecStressNGInVirtualMachine(f, vm)

		By("The first failed migration")
		const vmopName1 = "local-disks-migration-1"

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(vm, vmopbuilder.WithName(vmopName1))

		untilVirtualMachinesWillBeStartMigratingAndCancelImmediately(f)

		untilVirtualDisksMigrationsFailed(f)

		By("The second failed migration")
		const vmopName2 = "local-disks-migration-2"

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(vm, vmopbuilder.WithName(vmopName2))

		Eventually(func(g Gomega) {
			vmop, err := f.VirtClient().VirtualMachineOperations(ns).Get(context.Background(), vmopName2, metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
		}).WithTimeout(framework.MaxTimeout).WithPolling(time.Second).Should(Succeed())

		vm, err = f.VirtClient().VirtualMachines(ns).Get(context.Background(), vm.GetName(), metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.Status.MigrationState).ShouldNot(BeNil())
		Expect(vm.Status.MigrationState.EndTimestamp).ShouldNot(BeNil())
		Expect(vm.Status.MigrationState.Result).To(Equal(v1alpha2.MigrationResultSucceeded))

		untilVirtualDisksMigrationsSucceeded(f)
	})

	DescribeTable("should be reverted because virtual machine stopped", func(slap func(vm *v1alpha2.VirtualMachine) error) {
		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		ExecStressNGInVirtualMachine(f, vm)

		const vmopName = "local-disks-migration"

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(vm, vmopbuilder.WithName(vmopName))

		Eventually(func() error {
			vm, err = f.VirtClient().VirtualMachines(ns).Get(context.Background(), vm.GetName(), metav1.GetOptions{})
			if err != nil {
				return err
			}

			state := vm.Status.MigrationState

			readyToCancel := state != nil && !state.StartTimestamp.IsZero()
			if !readyToCancel {
				return fmt.Errorf("migration is not in progress")
			}

			return slap(vm)
		}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())

		untilVirtualDisksMigrationsFailed(f)
	},
		Entry("when virtual machine deleting", func(vm *v1alpha2.VirtualMachine) error {
			return f.VirtClient().VirtualMachines(vm.GetNamespace()).Delete(context.Background(), vm.GetName(), metav1.DeleteOptions{})
		}),
		Entry("when virtual machine stopped from OS", func(vm *v1alpha2.VirtualMachine) error {
			By(fmt.Sprintf("Exec shutdown command for virtualmachine %s/%s", vm.Namespace, vm.Name))
			return util.StopVirtualMachineFromOS(f, vm)
		}),
	)

	Context("Migrate to not matched node", func() {
		const (
			unknownLabelKey = "unknown-label-key"
		)

		nodeLabelAdd := func(node *corev1.Node) {
			GinkgoHelper()

			patchBytes := []byte(fmt.Sprintf(`{"metadata":{"labels": {"%s": "true"}}}`, unknownLabelKey))
			_, err := f.KubeClient().CoreV1().Nodes().Patch(context.Background(), node.GetName(), types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		nodeLabelDelete := func(node *corev1.Node) {
			GinkgoHelper()

			if _, ok := node.Labels[unknownLabelKey]; ok {
				newLabels := make(map[string]string, len(node.Labels))
				maps.Copy(newLabels, node.Labels)
				delete(newLabels, unknownLabelKey)

				patchBytes, err := patch.NewJSONPatch(patch.WithReplace("/metadata/labels", newLabels)).Bytes()
				Expect(err).NotTo(HaveOccurred())

				_, err = f.KubeClient().CoreV1().Nodes().Patch(context.Background(), node.GetName(), types.JSONPatchType, patchBytes, metav1.PatchOptions{})
				Expect(err).NotTo(HaveOccurred())
			}
		}

		BeforeEach(func() {
			nodes, err := f.KubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			for _, node := range nodes.Items {
				nodeLabelAdd(&node)
			}

			DeferCleanup(func() {
				for _, node := range nodes.Items {
					nodeLabelDelete(&node)
				}
			})
		})

		It("should reverted because migration canceled when pod pending", func() {
			ns := f.Namespace().Name

			vm, vds := localMigrationRootAndAdditionalBuild()
			vm.Spec.NodeSelector = map[string]string{unknownLabelKey: "true"}

			vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vm)

			for _, vd := range vds {
				_, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				f.DeferDelete(vd)
			}

			By("Wait until VM agent is ready")
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

			vm, err = f.VirtClient().VirtualMachines(ns).Get(context.Background(), vm.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			vmNodeName := vm.Status.Node
			Expect(vmNodeName).NotTo(BeEmpty())

			nodes, err := f.KubeClient().CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			for _, node := range nodes.Items {
				if node.GetName() != vmNodeName {
					nodeLabelDelete(&node)
				}
			}

			const vmopName = "local-disks-migration"

			By("Starting migrations for virtual machines")
			util.MigrateVirtualMachine(vm, vmopbuilder.WithName(vmopName))

			Eventually(func() error {
				pods, err := f.KubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
				Expect(err).NotTo(HaveOccurred())

				if len(pods.Items) != 2 {
					return fmt.Errorf("unexpected number of pods")
				}

				var (
					runningPod *corev1.Pod
					pendingPod *corev1.Pod
				)

				for _, pod := range pods.Items {
					switch pod.Status.Phase {
					case corev1.PodRunning:
						runningPod = &pod
					case corev1.PodPending:
						pendingPod = &pod
					}
				}

				if runningPod == nil || pendingPod == nil {
					return fmt.Errorf("unexpected pod phase")
				}

				scheduled, _ := conditions.GetPodCondition(corev1.PodScheduled, pendingPod.Status.Conditions)
				if scheduled.Status == corev1.ConditionFalse && scheduled.Reason == corev1.PodReasonUnschedulable {
					return nil
				}

				return fmt.Errorf("pending pod is not unschedulable")
			}).WithTimeout(framework.LongTimeout).WithPolling(time.Second).Should(Succeed())

			err = f.VirtClient().VirtualMachineOperations(ns).Delete(context.Background(), vmopName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			untilVirtualDisksMigrationsFailed(f)
		})
	})

	It("should be failed with RWO VMBDA", func() {
		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()

		By("Creating VM")
		vm, err := f.VirtClient().VirtualMachines(ns).Create(context.Background(), vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		By("Creating VDs")
		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(context.Background(), vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Creating RWO VD for VMBDA")
		const vdVmbdaName = "vd-vmbda-rwo"
		vdVmbda := object.NewBlankVD(vdVmbdaName, ns, &storageClass.Name, ptr.To(resource.MustParse("100Mi")))
		_, err = f.VirtClient().VirtualDisks(ns).Create(context.Background(), vdVmbda, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vdVmbda)

		By("Creating VMBDA")
		const vmbdaName = "vd-vmbda-rwo"
		vmbda := object.NewVMBDAFromDisk(vmbdaName, vm.Name, vdVmbda)
		_, err = f.VirtClient().VirtualMachineBlockDeviceAttachments(ns).Create(context.Background(), vmbda, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vmbda)

		By("Wait until VM agent is ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		const vmopName = "local-disks-migration-with-rwo-vmbda"

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(vm, vmopbuilder.WithName(vmopName))

		By("Waiting for migration failed")
		Eventually(func(g Gomega) {
			vmop, err := f.VirtClient().VirtualMachineOperations(ns).Get(context.Background(), vmopName, metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
			completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
			g.Expect(completed.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(completed.Reason).To(Equal(vmopcondition.ReasonHotplugDisksNotShared.String()))
		}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())
	})
})

func ExecStressNGInVirtualMachine(f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...framework.SSHCommandOption) {
	GinkgoHelper()

	cmd := "sudo nohup stress-ng --vm 1 --vm-bytes 100% --timeout 300s &>/dev/null &"

	By(fmt.Sprintf("Exec StressNG command for virtualmachine %s/%s", vm.Namespace, vm.Name))
	Expect(f.SSHCommand(vm.Name, vm.Namespace, cmd, options...)).To(Succeed())

	By("Wait until stress-ng loads the memory more heavily")
	time.Sleep(20 * time.Second)
}
