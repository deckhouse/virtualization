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

	It("supersedes a stuck Migrate operation with a Stop", func() {
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

		By("Supersede the Migrate with a Stop")
		stopVMOP := vmopbuilder.New(
			vmopbuilder.WithName("vmop-stop"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
			vmopbuilder.WithVirtualMachine(vm.Name),
		)
		err = f.CreateWithDeferredDeletion(ctx, stopVMOP)
		Expect(err).NotTo(HaveOccurred())

		By("Ensure the Migrate VMOP is Superseded")
		util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseSuperseded), framework.MiddleTimeout, migrateVMOP)
		util.UntilConditionReason(ctx, vmopcondition.TypeCompleted.String(), vmopcondition.ReasonSuperseded.String(), framework.MiddleTimeout, migrateVMOP)
	})
})
