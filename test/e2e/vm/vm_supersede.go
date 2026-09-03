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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// nonExistentNodeSelector pins a workload to a node that does not exist, so the
// launcher (or migration target) pod stays Unschedulable forever. This keeps the
// operation we want to supersede in a non-terminal phase without any timing race.
var nonExistentNodeSelector = map[string]string{"kubernetes.io/hostname": "non-existent-node"}

// supersedeVMOPInProgress reports the VMOP has entered the InProgress phase.
func supersedeVMOPInProgress() vmopobs.Predicate {
	return func(op *v1alpha2.VirtualMachineOperation) (bool, error) {
		return op.Status.Phase == v1alpha2.VMOPPhaseInProgress, nil
	}
}

// supersedeVMOPSuperseded reports the VMOP has been superseded: the phase is
// Superseded and the Completed condition carries the Superseded reason.
func supersedeVMOPSuperseded() vmopobs.Predicate {
	return func(op *v1alpha2.VirtualMachineOperation) (bool, error) {
		if op.Status.Phase != v1alpha2.VMOPPhaseSuperseded {
			return false, nil
		}
		for _, c := range op.Status.Conditions {
			if c.Type == vmopcondition.TypeCompleted.String() {
				if c.Reason == vmopcondition.ReasonSuperseded.String() {
					return true, nil
				}
				return false, fmt.Errorf("vmop %s/%s is Superseded but the Completed condition reason is %q, expected %q",
					op.Namespace, op.Name, c.Reason, vmopcondition.ReasonSuperseded)
			}
		}
		return false, nil
	}
}

var _ = Describe("VirtualMachineSupersede", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var f *framework.Framework

	It("supersedes a stuck Start operation with a Stop", func() {
		ctx := context.Background()
		f = framework.NewFramework("vm-supersede-start")
		// The launcher pod is deliberately parked Unschedulable by
		// nonExistentNodeSelector.
		f.TolerateUnschedulablePods()
		DeferCleanup(f.After)
		f.Before()

		vdRoot := object.NewBlankVD("vd-root", f.Namespace().Name, nil, ptr.To(resource.MustParse(vdCustomImageSize)))
		vm := object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			// The VM never boots here (its pod is unschedulable), so no
			// provisioning is needed.
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
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Start the VM: its launcher pod is unschedulable, so the Start VMOP stays InProgress")
		startVMOP := vmopbuilder.New(
			vmopbuilder.WithName("vmop-start"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
			vmopbuilder.WithVirtualMachine(vm.Name),
		)
		startVMOPObs := vmopobs.StartObserver(ctx, startVMOP)
		err = f.CreateWithDeferredDeletion(ctx, startVMOP)
		Expect(err).NotTo(HaveOccurred())
		err = startVMOPObs.WaitFor(supersedeVMOPInProgress(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())

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
		err = startVMOPObs.WaitFor(supersedeVMOPSuperseded(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable("supersedes a stuck Migrate operation", func(supersederType v1alpha2.VMOPType) {
		ctx := context.Background()
		f = framework.NewFramework("vm-supersede-migrate")
		// The migration target pod is deliberately parked Unschedulable by
		// nonExistentNodeSelector, and superseding the stuck Migrate VMOP
		// deliberately aborts the migration.
		f.TolerateUnschedulablePods()
		f.TolerateFailedMigrations()
		DeferCleanup(f.After)
		f.Before()

		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS,
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
		)
		vm := object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			// The custom image has no cloud-init; the guest agent is baked
			// in, so no provisioning is needed.
			vmbuilder.WithRunPolicy(v1alpha2.ManualPolicy),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			}),
		)

		By("Environment preparation: start the VM and wait until it is running")
		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vm)
		Expect(err).NotTo(HaveOccurred())
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		startVMOP := util.StartVirtualMachine(ctx, f, vm)
		startVMOPObs := vmopobs.StartObserver(ctx, startVMOP)
		err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		// The VM turns Running slightly before the Start VMOP turns Completed,
		// and the VMOP webhook rejects a new operation until the previous one
		// finishes.
		err = startVMOPObs.WaitFor(vmopobs.BeCompleted(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Migrate the VM to a non-existent node: the target pod is unschedulable, so the Migrate VMOP stays InProgress")
		migrateVMOP := vmopbuilder.New(
			vmopbuilder.WithName("vmop-migrate"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
			vmopbuilder.WithVirtualMachine(vm.Name),
			vmopbuilder.WithVMOPMigrateNodeSelector(nonExistentNodeSelector),
		)
		migrateVMOPObs := vmopobs.StartObserver(ctx, migrateVMOP)
		err = util.CreateVMOPRetryingStaleActiveDenial(ctx, f, migrateVMOP)
		Expect(err).NotTo(HaveOccurred())
		// Migration slot contention in parallel e2e runs can keep the VMOP
		// Pending for a while before the migration actually starts.
		err = migrateVMOPObs.WaitFor(supersedeVMOPInProgress(), framework.MaxTimeout)
		Expect(err).NotTo(HaveOccurred())

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
		err = migrateVMOPObs.WaitFor(supersedeVMOPSuperseded(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())
	},
		Entry("with a Stop", v1alpha2.VMOPTypeStop),
		Entry("with a Restart", v1alpha2.VMOPTypeRestart),
	)

	DescribeTable("supersedes a stuck Restart operation", func(supersederType v1alpha2.VMOPType) {
		ctx := context.Background()
		f = framework.NewFramework("vm-supersede-restart")
		// The restarted launcher pod is deliberately parked Unschedulable by
		// the oversized CPU topology.
		f.TolerateUnschedulablePods()
		DeferCleanup(f.After)
		f.Before()

		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomBIOS,
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
		)
		vm := object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm"),
			// The custom image has no cloud-init; the guest agent is baked
			// in, so no provisioning is needed.
			vmbuilder.WithRunPolicy(v1alpha2.ManualPolicy),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.VirtualDiskKind,
				Name: vdRoot.Name,
			}),
		)

		By("Environment preparation: start the VM and wait until it is running")
		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vm)
		Expect(err).NotTo(HaveOccurred())
		vmObs := vmobs.StartObserver(ctx, f, vm)
		err = vmObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		startVMOP := util.StartVirtualMachine(ctx, f, vm)
		startVMOPObs := vmopobs.StartObserver(ctx, startVMOP)
		err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		// The VM turns Running slightly before the Start VMOP turns Completed,
		// and the VMOP webhook rejects a new operation until the previous one
		// finishes.
		err = startVMOPObs.WaitFor(vmopobs.BeCompleted(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())

		By("Bump CPU cores beyond cluster capacity")
		// unschedulableCPUCores changes the CPU socket topology, which makes the
		// cores change restart-required rather than a live-migrating hotplug, and
		// exceeds any node's capacity, so the restarted launcher pod stays
		// Unschedulable and the Restart VMOP never leaves InProgress.
		// The core fraction is raised in the same patch: the sizing policy allows
		// only 100% at that core count. Raising it here rather than at creation
		// keeps the running VM at the custom-image sizing (50m) until the doomed
		// restart, whose pod is never scheduled and never consumes those cores.
		const unschedulableCPUCores = 240
		patch, err := json.Marshal([]map[string]interface{}{{
			"op":    "replace",
			"path":  "/spec/cpu/cores",
			"value": unschedulableCPUCores,
		}, {
			"op":    "replace",
			"path":  "/spec/cpu/coreFraction",
			"value": "100%",
		}})
		Expect(err).NotTo(HaveOccurred())
		err = f.GenericClient().Patch(ctx, vm, crclient.RawPatch(types.JSONPatchType, patch))
		Expect(err).NotTo(HaveOccurred())

		By("Restart the VM: the restarted pod is unschedulable, so the Restart VMOP stays InProgress")
		restartVMOP := vmopbuilder.New(
			vmopbuilder.WithName("vmop-restart"),
			vmopbuilder.WithNamespace(vm.Namespace),
			vmopbuilder.WithType(v1alpha2.VMOPTypeRestart),
			vmopbuilder.WithVirtualMachine(vm.Name),
		)
		restartVMOPObs := vmopobs.StartObserver(ctx, restartVMOP)
		err = util.CreateVMOPRetryingStaleActiveDenial(ctx, f, restartVMOP)
		Expect(err).NotTo(HaveOccurred())
		err = restartVMOPObs.WaitFor(supersedeVMOPInProgress(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())

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
		err = restartVMOPObs.WaitFor(supersedeVMOPSuperseded(), framework.MiddleTimeout)
		Expect(err).NotTo(HaveOccurred())
	},
		Entry("by a force Stop", v1alpha2.VMOPTypeStop),
		Entry("by a force Restart", v1alpha2.VMOPTypeRestart),
	)
})
