/*
Copyright 2026 Flant JSC

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// nonExistentNodeSelector pins a workload to a node that does not exist, so the
// launcher (or migration target) pod stays Unschedulable forever. This keeps the
// operation we want to supersede in a non-terminal phase without any timing race.
var nonExistentNodeSelector = map[string]string{"kubernetes.io/hostname": "non-existent-node"}

var _ = Describe("VirtualMachineSupersede", Label(precheck.NoPrecheck), func() {
	var f *framework.Framework

	It("supersedes a stuck Start operation with a Stop", func() {
		ctx := context.Background()
		f = framework.NewFramework("vm-supersede-start")
		DeferCleanup(f.After)
		f.Before()

		vdRoot := object.NewBlankVD("vd-root", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))
		vm := object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			vmbuilder.WithRunPolicy(v1alpha2.ManualPolicy),
			vmbuilder.WithNodeSelector(nonExistentNodeSelector),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			}),
		)

		By("Environment preparation")
		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vm)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineStopped), framework.LongTimeout, vm)

		By("Start the VM: its launcher pod is unschedulable, so the Start VMOP stays InProgress")
		startVMOP := vmopbuilder.New(
			vmopbuilder.WithName("vmop-start"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
			vmopbuilder.WithVirtualMachine(vm.Name),
		)
		err = f.CreateWithDeferredDeletion(ctx, startVMOP)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseInProgress), framework.MiddleTimeout, startVMOP)

		By("Supersede the Start with a Stop")
		stopVMOP := vmopbuilder.New(
			vmopbuilder.WithName("vmop-stop"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
			vmopbuilder.WithVirtualMachine(vm.Name),
		)
		err = f.CreateWithDeferredDeletion(ctx, stopVMOP)
		Expect(err).NotTo(HaveOccurred())

		By("Ensure the Start VMOP is Superseded")
		util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseSuperseded), framework.MiddleTimeout, startVMOP)
		util.UntilConditionReason(ctx, vmopcondition.TypeCompleted.String(), vmopcondition.ReasonSuperseded.String(), framework.MiddleTimeout, startVMOP)
	})

	DescribeTable("supersedes a stuck Migrate operation", func(supersederType v1alpha2.VMOPType) {
		ctx := context.Background()
		f = framework.NewFramework("vm-supersede-migrate")
		DeferCleanup(f.After)
		f.Before()

		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIUbuntu)
		vm := object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			vmbuilder.WithRunPolicy(v1alpha2.ManualPolicy),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			}),
		)

		By("Environment preparation: start the VM and wait until it is running")
		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vm)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineStopped), framework.LongTimeout, vm)
		util.StartVirtualMachine(ctx, f, vm)
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)

		By("Migrate the VM to a non-existent node: the target pod is unschedulable, so the Migrate VMOP stays InProgress")
		migrateVMOP := vmopbuilder.New(
			vmopbuilder.WithName("vmop-migrate"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
			vmopbuilder.WithVirtualMachine(vm.Name),
			vmopbuilder.WithVMOPMigrateNodeSelector(nonExistentNodeSelector),
		)
		err = f.CreateWithDeferredDeletion(ctx, migrateVMOP)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseInProgress), framework.MiddleTimeout, migrateVMOP)

		By("Supersede the Migrate with a " + string(supersederType))
		superseder := vmopbuilder.New(
			vmopbuilder.WithName("vmop-superseder"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(supersederType),
			vmopbuilder.WithVirtualMachine(vm.Name),
		)
		err = f.CreateWithDeferredDeletion(ctx, superseder)
		Expect(err).NotTo(HaveOccurred())

		By("Ensure the Migrate VMOP is Superseded")
		util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseSuperseded), framework.MiddleTimeout, migrateVMOP)
		util.UntilConditionReason(ctx, vmopcondition.TypeCompleted.String(), vmopcondition.ReasonSuperseded.String(), framework.MiddleTimeout, migrateVMOP)
	},
		Entry("with a Stop", v1alpha2.VMOPTypeStop),
		Entry("with a Restart", v1alpha2.VMOPTypeRestart),
	)

	DescribeTable("supersedes a stuck Restart operation", func(supersederType v1alpha2.VMOPType) {
		ctx := context.Background()
		f = framework.NewFramework("vm-supersede-restart")
		DeferCleanup(f.After)
		f.Before()

		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVIUbuntu)
		vm := object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			vmbuilder.WithRunPolicy(v1alpha2.ManualPolicy),
			// 100% core fraction is the only value the sizing policy allows at high
			// core counts, so set it up front to keep the later cores bump valid.
			vmbuilder.WithCPU(1, ptr.To("100%")),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			}),
		)

		By("Environment preparation: start the VM and wait until it is running")
		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vm)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineStopped), framework.LongTimeout, vm)
		util.StartVirtualMachine(ctx, f, vm)
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)

		By("Bump CPU cores beyond cluster capacity")
		// unschedulableCPUCores changes the CPU socket topology, which makes the
		// cores change restart-required rather than a live-migrating hotplug, and
		// exceeds any node's capacity, so the restarted launcher pod stays
		// Unschedulable and the Restart VMOP never leaves InProgress.
		const unschedulableCPUCores = 240
		running := getVirtualMachine(ctx, f, vm.Name)
		running.Spec.CPU.Cores = unschedulableCPUCores
		Expect(f.GenericClient().Update(ctx, running)).To(Succeed())

		By("Restart the VM: the restarted pod is unschedulable, so the Restart VMOP stays InProgress")
		restartVMOP := vmopbuilder.New(
			vmopbuilder.WithName("vmop-restart"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeRestart),
			vmopbuilder.WithVirtualMachine(vm.Name),
		)
		err = f.CreateWithDeferredDeletion(ctx, restartVMOP)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseInProgress), framework.MiddleTimeout, restartVMOP)

		By("Supersede the Restart with a forced " + string(supersederType))
		superseder := vmopbuilder.New(
			vmopbuilder.WithName("vmop-superseder"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(supersederType),
			vmopbuilder.WithVirtualMachine(vm.Name),
			vmopbuilder.WithForce(ptr.To(true)),
		)
		err = f.CreateWithDeferredDeletion(ctx, superseder)
		Expect(err).NotTo(HaveOccurred())

		By("Ensure the Restart VMOP is Superseded")
		util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseSuperseded), framework.MiddleTimeout, restartVMOP)
		util.UntilConditionReason(ctx, vmopcondition.TypeCompleted.String(), vmopcondition.ReasonSuperseded.String(), framework.MiddleTimeout, restartVMOP)
	},
		Entry("by a force Stop", v1alpha2.VMOPTypeStop),
		Entry("by a force Restart", v1alpha2.VMOPTypeRestart),
	)
})
