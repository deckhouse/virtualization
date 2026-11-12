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

package release

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// One spec per virtual machine; the objects come from Prepare.
var _ = Describe("ModuleUpgradeVirtualMachine", Label(label.SIGCompute), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		f = framework.NewFramework("")
		ctx = context.Background()
	})

	for _, s := range vmScenarios {
		It(fmt.Sprintf("%s survives the module upgrade", s.name), func() {
			switch GetPhase() {
			case PhasePreUpgrade:
				expectVMReadyBeforeUpgrade(ctx, f, s)
			case PhasePostUpgrade:
				expectVMSurvivedUpgrade(ctx, f, s)
			}
		})
	}
})

func expectVMReadyBeforeUpgrade(ctx context.Context, f *framework.Framework, s vmScenario) {
	GinkgoHelper()

	vm := s.newVMRef()
	if s.runPolicy == v1alpha2.ManualPolicy {
		By("Waiting for the manual-policy virtual machine to be Stopped")
		util.UntilObjectPhase(ctx, string(v1alpha2.MachineStopped), framework.MiddleTimeout, vm)

		By("Starting it with a VirtualMachineOperation")
		util.StartVirtualMachine(ctx, f, vm)
	}

	By("Waiting for the virtual machine to be Running")

	util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)

	disks := findDataDisks(s.name)
	if len(disks) > 0 {
		By("Waiting for the hotplug attachments to be Attached")
		util.UntilObjectPhase(ctx, string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MaxTimeout, listAttachmentRefs(disks)...)
	}

	By("Waiting for the disks to be Ready")
	util.UntilObjectPhase(ctx, string(v1alpha2.DiskReady), framework.LongTimeout, listDiskRefs(s)...)

	expectGuestSeesDisks(f, vm, len(disks))
}

func expectVMSurvivedUpgrade(ctx context.Context, f *framework.Framework, s vmScenario) {
	GinkgoHelper()

	vm := s.newVMRef()

	By("Waiting for the virtual machine to be Running")
	util.UntilObjectPhase(ctx, string(v1alpha2.MachineRunning), framework.LongTimeout, vm)

	expectGuestSeesDisks(f, vm, len(findDataDisks(s.name)))

	if !migratesVMs() {
		By(fmt.Sprintf("Skipping the restart check: %s=false, the upgrade does not migrate virtual machines", migratesVMsEnv))
		return
	}

	By("Verifying the virtual machine was live-migrated and not restarted")
	window := findMigrationWindow(ctx, f, vm)
	current := getVirtualMachine(ctx, f, vm)
	Expect(current.Status.Stats).NotTo(BeNil(), "virtual machine %s/%s has no stats", vm.Namespace, vm.Name)
	Expect(current.Status.Stats.LastStartTime).NotTo(BeNil(), "virtual machine %s/%s has no last start time", vm.Namespace, vm.Name)
	Expect(current.Status.Stats.LastStartTime.Time).To(BeTemporally("<", window.start),
		"virtual machine %s/%s restarted during the module upgrade: last start %s, migration started %s",
		vm.Namespace, vm.Name, current.Status.Stats.LastStartTime.Time, window.start)
}

// The guest sees its root disk plus the hotplugged data disks; the custom
// image has no cloud-init disk.
func expectGuestSeesDisks(f *framework.Framework, vm *v1alpha2.VirtualMachine, dataDisks int) {
	GinkgoHelper()

	By("Waiting for SSH access to the guest")
	eventually.SSHReadyAsRoot(f, vm, framework.LongTimeout)

	By("Checking the disks inside the guest")
	expected := 1 + dataDisks
	eventually.UntilDiskCountAsRoot(f, vm.Name, vm.Namespace, Equal(expected), framework.LongTimeout,
		eventually.WithExplanation("the guest of %s/%s must see its root disk and %d hotplugged disks", vm.Namespace, vm.Name, dataDisks))
}
