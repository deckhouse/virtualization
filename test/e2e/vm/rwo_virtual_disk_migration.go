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

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// migratableVDs returns the subset of disks that a live migration actually
// moves: only local (ReadWriteOnce) disks take part in a volume migration,
// replicated disks stay in place.
func migratableVDs(vds []*v1alpha2.VirtualDisk) []*v1alpha2.VirtualDisk {
	var out []*v1alpha2.VirtualDisk
	for _, vd := range vds {
		if vd.Annotations[annotations.AnnVirtualDiskAccessMode] == "ReadWriteOnce" {
			out = append(out, vd)
		}
	}
	return out
}

var _ = Describe("RWOVirtualDiskMigration", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f            *framework.Framework
		ctx          context.Context
		storageClass *storagev1.StorageClass
		vi           *v1alpha2.VirtualImage
	)

	BeforeEach(func() {
		// TODO: Re-enable the suite.
		Skip("skipped as flaky: fix the instability, then remove this skip")

		ctx = context.Background()
		f = framework.NewFramework("rwo-virtual-disk-migration")
		storageClass = framework.GetConfig().StorageClass.DefaultStorageClass
		if storageClass == nil {
			Skip("DefaultStorageClass is not set.")
		}

		f.Before()

		DeferCleanup(f.After)

		// The custom image bakes in stress-ng, which the revert/cancel
		// specs use to keep the migration running long enough to cancel it.
		newVI := object.NewGeneratedVIFromCVI("rwo-virtual-disk-migration-", f.Namespace().Name, object.PrecreatedCVICustomBIOS)
		newVI, err := f.VirtClient().VirtualImages(f.Namespace().Name).Create(ctx, newVI, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(newVI)
		vi = newVI
	})

	const (
		vdRootName       = "vd-alpine-root-disk"
		vdAdditionalName = "vd-alpine-additional-disk"
	)

	localMigrationRootOnlyBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return onlyRootBuild(f, vi, buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: true, size: vdCustomImageSize})
	}

	localMigrationRootAndAdditionalBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return rootAndAdditionalBuild(f, vi,
			buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: true, size: vdCustomImageSize},
			buildOption{name: vdAdditionalName, storageClass: &storageClass.Name, rwo: true, size: vdCustomImageSize},
		)
	}

	localMigrationAdditionalOnlyBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return onlyAdditionalBuild(f, vi,
			buildOption{name: vdRootName, rwo: false, size: vdCustomImageSize},
			buildOption{name: vdAdditionalName, rwo: true, size: vdCustomImageSize},
		)
	}

	localMigrationManyDisksBuild := func() (*v1alpha2.VirtualMachine, []*v1alpha2.VirtualDisk) {
		return rootAndManyAdditionalBuild(f, vi, buildOption{name: vdRootName, storageClass: &storageClass.Name, rwo: true, size: vdCustomImageSize}, &storageClass.Name, 3)
	}

	DescribeTable("should be successful", func(build func() (vm *v1alpha2.VirtualMachine, vds []*v1alpha2.VirtualDisk)) {
		ns := f.Namespace().Name

		vm, vds := build()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		const vmopName = "local-disks-migration"

		// Armed before the trigger so no migration transition is missed.
		migratable := migratableVDs(vds)
		vdObservers := startVDObservers(ctx, f, migratable...)

		By("Starting migrations for virtual machines")
		vmop := util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName))

		util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)

		// The VM's status.migrationState is synced asynchronously after the
		// VMOP completes, so wait for it instead of asserting a one-shot read.
		err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())

		expectVDsMigrationSucceeded(ctx, f, vdObservers, migratable...)
	},
		Entry("when only root disk on local storage", localMigrationRootOnlyBuild),
		Entry("when root disk on local storage and one additional disk", localMigrationRootAndAdditionalBuild),
		// TODO: rnd and uncomment when problem will be solved
		// Entry("when only additional disk on local storage", localMigrationAdditionalOnlyBuild),
	)

	// LabelTolerateFailedMigrations: the cancel deliberately terminates the
	// volume migration with a failure, which the framework's failed-migration
	// fail-fast rules would otherwise abort the spec on.
	DescribeTable("should be reverted", Label(framework.LabelTolerateFailedMigrations), func(build func() (vm *v1alpha2.VirtualMachine, vds []*v1alpha2.VirtualDisk)) {
		ns := f.Namespace().Name

		vm, vds := build()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		ExecStressNGInVirtualMachine(f, vm)

		const vmopName = "local-disks-migration"

		// Armed before the trigger so no migration transition is missed.
		migratable := migratableVDs(vds)
		vdObservers := startVDObservers(ctx, f, migratable...)

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName))

		untilVirtualMachinesWillBeStartMigratingAndCancelImmediately(f)

		expectVDsMigrationFailed(ctx, f, vdObservers, migratable...)
	},
		Entry("when only root disk on local storage", localMigrationRootOnlyBuild),
		Entry("when root disk on local storage and one additional disk", localMigrationRootAndAdditionalBuild),
		Entry("when only additional disk on local storage", localMigrationAdditionalOnlyBuild),
	)

	It("should be successful two migrations in a row", func() {
		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		migratable := migratableVDs(vds)

		for i := range 2 {
			vmopName := "local-disks-migration-" + strconv.Itoa(i)

			// Armed before each round's trigger so its transitions are captured.
			vdObservers := startVDObservers(ctx, f, migratable...)

			By("Starting migrations for virtual machines")
			vmop := util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName))

			util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)

			// The VM's status.migrationState is synced asynchronously after the
			// VMOP completes, so wait for it instead of asserting a one-shot read.
			err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MiddleTimeout)
			Expect(err).NotTo(HaveOccurred())

			expectVDsMigrationSucceeded(ctx, f, vdObservers, migratable...)
		}
	})

	It("keeps a multi-disk volume set consistent when a restart is requested mid-migration", func() {
		ns := f.Namespace().Name

		vm, vds := localMigrationManyDisksBuild()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		migratable := migratableVDs(vds)

		By("Migrating the whole set once so the disks move off their base PVCs")
		firstObservers := startVDObservers(ctx, f, migratable...)
		firstVMOP := util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName("many-disks-migration-1"))
		util.UntilVMOPMigrationSucceeded(ctx, firstVMOP, framework.MaxTimeout)
		expectVDsMigrationSucceeded(ctx, f, firstObservers, migratable...)

		By("Starting a second migration of the whole volume set")
		secondObservers := startVDObservers(ctx, f, migratable...)
		vmop := util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName("many-disks-migration-2"))

		// Request a restart right after the migration starts, so the restart reconcile
		// races with the still-unfinalized volume set. It must not issue a conflicting
		// volume update over that set, otherwise KubeVirt rejects it ("the volume can only
		// be reverted to the previous version during the update") and the set is left
		// inconsistent. On copy-based storage the patch lands mid-migration; on instant
		// (replicated) storage it lands right after — both must finalize cleanly.
		By("Requesting a restart around the migration")
		patchBytes, err := patch.NewJSONPatch(patch.WithAdd("/spec/terminationGracePeriodSeconds", int64(11))).Bytes()
		Expect(err).NotTo(HaveOccurred())
		_, err = f.VirtClient().VirtualMachines(ns).Patch(ctx, vm.GetName(), types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("The migration still finalizes cleanly")
		util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)

		// The VM's status.migrationState is synced asynchronously after the
		// VMOP completes, so wait for it instead of asserting a one-shot read.
		err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())

		expectVDsMigrationSucceeded(ctx, f, secondObservers, migratable...)

		By("Restart stays pending: the change was neither lost nor applied without a restart")
		Expect(util.IsRestartRequired(vm, framework.ShortTimeout)).To(BeTrue())
	})

	It("should be successful when a restart is pending", func() {
		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()
		vmbuilder.ApplyOptions(vm, []vmbuilder.Option{vmbuilder.WithRestartApprovalMode(v1alpha2.Manual)})

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Applying a change that requires a restart")
		patchBytes, err := patch.NewJSONPatch(patch.WithAdd("/spec/terminationGracePeriodSeconds", int64(11))).Bytes()
		Expect(err).NotTo(HaveOccurred())
		vm, err = f.VirtClient().VirtualMachines(ns).Patch(ctx, vm.GetName(), types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(util.IsRestartRequired(vm, framework.ShortTimeout)).To(BeTrue())

		const vmopName = "local-disks-migration-under-restart"

		migratable := migratableVDs(vds)
		vdObservers := startVDObservers(ctx, f, migratable...)

		By("Starting migration while the restart is still pending")
		vmop := util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName))

		util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)

		// The VM's status.migrationState is synced asynchronously after the
		// VMOP completes, so wait for it instead of asserting a one-shot read.
		err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())

		expectVDsMigrationSucceeded(ctx, f, vdObservers, migratable...)

		By("Restart is still pending after migration: the change was neither lost nor applied without a restart")
		Expect(util.IsRestartRequired(vm, framework.ShortTimeout)).To(BeTrue())
	})

	// LabelTolerateFailedMigrations: the first migration is deliberately
	// cancelled, so the failed-migration fail-fast rules must stay out.
	It("should be reverted first and completed second", Label(framework.LabelTolerateFailedMigrations), func() {
		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		ExecStressNGInVirtualMachine(f, vm)

		migratable := migratableVDs(vds)

		By("The first failed migration")
		const vmopName1 = "local-disks-migration-1"

		failedObservers := startVDObservers(ctx, f, migratable...)

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName1))

		untilVirtualMachinesWillBeStartMigratingAndCancelImmediately(f)

		expectVDsMigrationFailed(ctx, f, failedObservers, migratable...)

		By("The second completed migration")
		const vmopName2 = "local-disks-migration-2"

		succeededObservers := startVDObservers(ctx, f, migratable...)

		By("Starting migrations for virtual machines")
		vmop := util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName2))

		util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)

		// The VM's status.migrationState is synced asynchronously after the
		// VMOP completes, so wait for it instead of asserting a one-shot read.
		err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())

		expectVDsMigrationSucceeded(ctx, f, succeededObservers, migratable...)
	})

	// LabelTolerateFailedMigrations: interrupting the VM mid-migration
	// deliberately fails the volume migration.
	DescribeTable("should be reverted because virtual machine stopped", Label(framework.LabelTolerateFailedMigrations), func(slap func(vm *v1alpha2.VirtualMachine) error) {
		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()

		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Wait until VM agent is ready")
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		ExecStressNGInVirtualMachine(f, vm)

		const vmopName = "local-disks-migration"

		migratable := migratableVDs(vds)
		vdObservers := startVDObservers(ctx, f, migratable...)

		By("Starting migrations for virtual machines")
		util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName))

		By("Interrupting the virtual machine as soon as the migration starts")
		err = vmObs.WaitFor(vmobs.HaveMigrationStarted(), framework.MaxTimeout)
		Expect(err).NotTo(HaveOccurred())
		Expect(slap(vm)).To(Succeed())

		expectVDsMigrationFailed(ctx, f, vdObservers, migratable...)
	},
		Entry("when virtual machine deleting", func(vm *v1alpha2.VirtualMachine) error {
			return f.VirtClient().VirtualMachines(vm.GetNamespace()).Delete(ctx, vm.GetName(), metav1.DeleteOptions{})
		}),
		// Disabled because vm stopped after migration, that's why test fails.
		// Entry("when virtual machine stopped from OS", func(vm *v1alpha2.VirtualMachine) error {
		//	By(fmt.Sprintf("Exec shutdown command for virtualmachine %s/%s", vm.Namespace, vm.Name))
		//	util.StopVirtualMachineFromOS(f, vm)
		//	return nil
		// }),
	)

	// The target pod is deliberately parked Unschedulable and the migration is
	// then cancelled, so both fail-fast opt-outs apply to the whole context.
	Context("Migrate to not matched node", Label(framework.LabelTolerateUnschedulablePods, framework.LabelTolerateFailedMigrations), func() {
		const (
			unknownLabelKey = "unknown-label-key"
		)

		nodeLabelAdd := func(node *corev1.Node) {
			GinkgoHelper()

			patchBytes := []byte(fmt.Sprintf(`{"metadata":{"labels": {"%s": "true"}}}`, unknownLabelKey))
			_, err := f.KubeClient().CoreV1().Nodes().Patch(ctx, node.GetName(), types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
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

				_, err = f.KubeClient().CoreV1().Nodes().Patch(ctx, node.GetName(), types.JSONPatchType, patchBytes, metav1.PatchOptions{})
				Expect(err).NotTo(HaveOccurred())
			}
		}

		BeforeEach(func() {
			nodes, err := f.KubeClient().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
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

			vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vm)

			for _, vd := range vds {
				_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
				f.DeferDelete(vd)
			}

			By("Wait until VM agent is ready")
			vmObs := vmobs.StartObserver(ctx, f, vm)
			err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())

			vm, err = f.VirtClient().VirtualMachines(ns).Get(ctx, vm.GetName(), metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			vmNodeName := vm.Status.Node
			Expect(vmNodeName).NotTo(BeEmpty())

			nodes, err := f.KubeClient().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			for _, node := range nodes.Items {
				if node.GetName() != vmNodeName {
					nodeLabelDelete(&node)
				}
			}

			const vmopName = "local-disks-migration"

			migratable := migratableVDs(vds)
			vdObservers := startVDObservers(ctx, f, migratable...)

			By("Starting migrations for virtual machines")
			util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName))

			// The target virt-launcher pod is created by KubeVirt with a generated
			// name the test cannot know in advance, so it is discovered by watching
			// the namespace's pods for a virt-launcher pod (hp pods for hotplug
			// volumes carry a different label value) parked in Pending with the
			// Unschedulable reason.
			_, waitErr := observer.WaitForFirst(ctx,
				f.KubeClient().CoreV1().Pods(ns),
				framework.MaxTimeout,
				func(pod *corev1.Pod) bool {
					if pod.Labels["kubevirt.internal.virtualization.deckhouse.io"] != "virt-launcher" {
						return false
					}
					if pod.Status.Phase != corev1.PodPending {
						return false
					}
					scheduled, _ := conditions.GetPodCondition(corev1.PodScheduled, pod.Status.Conditions)
					return scheduled.Status == corev1.ConditionFalse && scheduled.Reason == corev1.PodReasonUnschedulable
				})
			Expect(waitErr).NotTo(HaveOccurred(), "the target virt-launcher pod should park in Pending/Unschedulable")

			err = f.VirtClient().VirtualMachineOperations(ns).Delete(ctx, vmopName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			expectVDsMigrationFailed(ctx, f, vdObservers, migratable...)
		})
	})

	It("should succeed with hotplugged RWO disk", func() {
		// TODO: unsupported combination — a VM with a VMBDA-hotplug-attached RWO
		// (node-local) disk cannot be volume-migrated. The hotplug volume is
		// attached only to the internal VMI (via the KubeVirt AddVolume
		// subresource), never to the VM template, so the volume-migration
		// readiness check (kvvmInCluster.template.volumes == kvvmi.volumes) can
		// never pass; the hotplug disk is also excluded from the migration disk
		// set (built from spec/status BlockDeviceRefs), so its node-local volume
		// is never migrated to the target node and the migration deadlocks.
		// Supporting it requires the controller to migrate VMBDA-attached RWO
		// volumes as part of the VM migration. Re-enable once implemented.
		Skip("volume migration of a VM with a VMBDA-hotplugged RWO node-local disk is not supported yet (hotplug volume is not part of the VM template / migration set)")

		ns := f.Namespace().Name

		vm, vds := localMigrationRootAndAdditionalBuild()

		By("Creating VM")
		vm, err := f.VirtClient().VirtualMachines(ns).Create(ctx, vm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vm)

		By("Creating VDs")
		for _, vd := range vds {
			_, err := f.VirtClient().VirtualDisks(ns).Create(ctx, vd, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vd)
		}

		By("Creating RWO VD for VMBDA")
		const vdVmbdaName = "vd-vmbda-rwo"
		vdVmbda := object.NewBlankVD(vdVmbdaName, ns, &storageClass.Name, ptr.To(resource.MustParse("50Mi")))
		_, err = f.VirtClient().VirtualDisks(ns).Create(ctx, vdVmbda, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vdVmbda)

		By("Creating VMBDA")
		const vmbdaName = "vd-vmbda-rwo"
		vmbda := object.NewVMBDAFromDisk(vmbdaName, vm.Name, vdVmbda)
		_, err = f.VirtClient().VirtualMachineBlockDeviceAttachments(ns).Create(ctx, vmbda, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(vmbda)

		By("Wait until VM agent is ready")
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		const vmopName = "local-disks-migration-with-rwo-vmbda"

		By("Starting migrations for virtual machines")
		vmop := util.MigrateVirtualMachine(f, vm, vmopbuilder.WithName(vmopName))
		util.UntilVMOPMigrationSucceeded(ctx, vmop, framework.MaxTimeout)

		// The VM's status.migrationState is synced asynchronously after the
		// VMOP completes, so wait for it instead of asserting a one-shot read.
		err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())
	})
})

func ExecStressNGInVirtualMachine(f *framework.Framework, vm *v1alpha2.VirtualMachine, options ...framework.SSHCommandOption) {
	GinkgoHelper()

	// The custom guest is accessed as root (no sudo) and runs POSIX sh
	// (no bash "&>" redirection); stress-ng is baked into the image.
	//
	// Half the guest's memory, not all of it. The point is to keep dirtying
	// pages so a migration has something to re-copy, and half of it does that
	// just as well - while asking for all of it leaves the guest of these
	// deliberately small VMs with a couple of free megabytes, one balloon
	// reclaim away from an out-of-memory kill that takes the guest down mid
	// migration.
	cmd := "nohup stress-ng --vm 1 --vm-bytes 50% --timeout 300s >/dev/null 2>&1 &"

	By(fmt.Sprintf("Exec StressNG command for virtualmachine %s/%s", vm.Namespace, vm.Name))
	opts := append([]framework.SSHCommandOption{framework.WithSSHUser("root")}, options...)
	_, err := f.SSHCommand(vm.Name, vm.Namespace, cmd, opts...)
	Expect(err).NotTo(HaveOccurred())

	By("Wait until stress-ng loads the memory more heavily")
	time.Sleep(20 * time.Second)
}
