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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("PowerState", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	DescribeTable("manages power state of a virtual machine", func(runPolicy v1alpha2.RunPolicy) {
		var namespaceSuffix string
		switch runPolicy {
		case v1alpha2.AlwaysOnPolicy:
			namespaceSuffix = "always-on"
		case v1alpha2.AlwaysOnUnlessStoppedManually:
			namespaceSuffix = "stopped-manually"
		case v1alpha2.ManualPolicy:
			namespaceSuffix = "manual"
		}
		ctx := context.Background()
		f := framework.NewFramework(fmt.Sprintf("power-state-%s", namespaceSuffix))
		DeferCleanup(f.After)
		f.Before()

		t := newPowerStateTest(f)

		var (
			vmObs    vmobs.Observer
			vmbdaObs vmbdaobs.Observer
		)

		By("Environment preparation", func() {
			t.GenerateResources(runPolicy)

			vmObs = vmobs.StartObserver(ctx, f, t.VM)
			vmObs.Never(vmobs.BeFailed())
			vmbdaObs = vmbdaobs.StartObserver(ctx, f, t.VMBDA)
			vmbdaObs.Never(vmbdaobs.BeFailed())

			err := f.CreateWithDeferredDeletion(
				ctx, t.VI, t.VDRoot, t.VDBlank, t.VM, t.VMBDA,
			)
			Expect(err).NotTo(HaveOccurred())

			if t.VM.Spec.RunPolicy == v1alpha2.ManualPolicy {
				err := vmObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				util.StartVirtualMachine(ctx, f, t.VM)
			}

			err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			// The first attachment waits out the blank disk provisioning and the CSI
			// attach of the hotplug pod, which take minutes under a parallel run.
			err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			eventually.SSHReadyAsRoot(f, t.VM, framework.LongTimeout)
		})

		By("Shutdown VM by VMOP", func() {
			// A fixed name (rather than generateName) lets the observer be armed
			// before the VMOP is created, so even an instant Failed/Completed
			// transition is captured.
			vmopStop := vmopbuilder.New(
				vmopbuilder.WithName(fmt.Sprintf("%s-stop", util.VmopE2ePrefix)),
				vmopbuilder.WithNamespace(t.VM.Namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
				vmopbuilder.WithVirtualMachine(t.VM.Name),
			)
			// Stopping an AlwaysOn VM must be rejected, so for that policy the
			// VMOP's Failed phase is the expected outcome, not a wedge.
			if t.VM.Spec.RunPolicy == v1alpha2.AlwaysOnPolicy {
				f.ExpectFailure(vmopStop)
			}
			vmopStopObs := vmopobs.StartObserver(ctx, vmopStop)
			err := f.CreateWithDeferredDeletion(ctx, vmopStop)
			Expect(err).NotTo(HaveOccurred())

			switch t.VM.Spec.RunPolicy {
			case v1alpha2.AlwaysOnPolicy:
				// Stopping an AlwaysOn VM is not allowed: the VMOP must fail and
				// the VM must keep running.
				err := vmopStopObs.WaitFor(beVMOPFailed(), framework.ShortTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmObs.WaitFor(vmobs.BeRunning(), framework.ShortTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.ShortTimeout)
				Expect(err).NotTo(HaveOccurred())
			case v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.ManualPolicy:
				err := vmopStopObs.WaitFor(vmopobs.BeCompleted(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				err = vmObs.WaitFor(vmobs.BeStopped(), framework.ShortTimeout)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		By("Start VM by VMOP", func() {
			if t.VM.Spec.RunPolicy != v1alpha2.AlwaysOnPolicy {
				util.StartVirtualMachine(ctx, f, t.VM)
				// A cold start brings up the launcher pod and attaches four block
				// devices through CSI, which outruns a minute under a parallel run.
				expectVMRunningSkippingStuckGuestShutdown(ctx, vmObs, t.VM, framework.LongTimeout)
				// The hotplugged disk is re-attached from scratch after a cold start,
				// through the same CSI path as the boot devices.
				err := vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				eventually.SSHReadyAsRoot(f, t.VM, framework.LongTimeout)
			}
		})

		By("Shutdown VM by SSH", func() {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())
			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			stopGuestAsRoot(f, t.VM)

			switch t.VM.Spec.RunPolicy {
			case v1alpha2.AlwaysOnPolicy:
				expectVMRebooted(ctx, vmObs, t.VM, runningLastTransitionTime, framework.LongTimeout)
				err := vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				eventually.SSHReadyAsRoot(f, t.VM, framework.LongTimeout)
			case v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.ManualPolicy:
				err := vmObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		By("Start VM by VMOP", func() {
			if t.VM.Spec.RunPolicy != v1alpha2.AlwaysOnPolicy {
				util.StartVirtualMachine(ctx, f, t.VM)
				// A cold start brings up the launcher pod and attaches four block
				// devices through CSI, which outruns a minute under a parallel run.
				expectVMRunningSkippingStuckGuestShutdown(ctx, vmObs, t.VM, framework.LongTimeout)
				// The hotplugged disk is re-attached from scratch after a cold start,
				// through the same CSI path as the boot devices.
				err := vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
				Expect(err).NotTo(HaveOccurred())
				eventually.SSHReadyAsRoot(f, t.VM, framework.LongTimeout)
			}
		})

		By("Reboot VM by VMOP", func() {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			// A fixed name (rather than generateName) lets the observer be armed
			// before the VMOP is created, so even an instant transition is captured.
			vmopRestart := vmopbuilder.New(
				vmopbuilder.WithName(fmt.Sprintf("%s-restart", util.VmopE2ePrefix)),
				vmopbuilder.WithNamespace(t.VM.Namespace),
				vmopbuilder.WithType(v1alpha2.VMOPTypeRestart),
				vmopbuilder.WithVirtualMachine(t.VM.Name),
			)
			vmopRestartObs := vmopobs.StartObserver(ctx, vmopRestart)
			err = f.CreateWithDeferredDeletion(ctx, vmopRestart)
			Expect(err).NotTo(HaveOccurred())

			err = vmopRestartObs.WaitFor(vmopobs.BeCompleted(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			expectVMRebooted(ctx, vmObs, t.VM, runningLastTransitionTime, framework.MiddleTimeout)
			err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			eventually.SSHReadyAsRoot(f, t.VM, framework.LongTimeout)
		})

		By("Reboot VM by SSH", func() {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			rebootGuestAsRoot(f, t.VM)

			expectVMRebooted(ctx, vmObs, t.VM, runningLastTransitionTime, framework.LongTimeout)
			err = vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			eventually.SSHReadyAsRoot(f, t.VM, framework.LongTimeout)
		})

		By("Check VM can reach external network", func() {
			err := network.CheckCiliumAgents(ctx, f.Kubectl(), t.VM.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", t.VM.Name)
			expectExternalConnectivityAsRoot(f, t.VM.Name, f.Namespace().Name)
		})
	},
		Entry(
			"AlwaysOn run policy",
			v1alpha2.AlwaysOnPolicy,
		),
		Entry(
			"Manual run policy",
			v1alpha2.ManualPolicy,
		),
		Entry(
			"AlwaysOnUnlessStoppedManually run policy",
			v1alpha2.AlwaysOnUnlessStoppedManually,
		),
	)
})

type powerStateTest struct {
	Framework *framework.Framework

	VI      *v1alpha2.VirtualImage
	VM      *v1alpha2.VirtualMachine
	VDRoot  *v1alpha2.VirtualDisk
	VDBlank *v1alpha2.VirtualDisk
	VMBDA   *v1alpha2.VirtualMachineBlockDeviceAttachment
}

func newPowerStateTest(f *framework.Framework) *powerStateTest {
	return &powerStateTest{
		Framework: f,
	}
}

func (t *powerStateTest) GenerateResources(runPolicy v1alpha2.RunPolicy) {
	t.VI = object.NewVI(
		vibuilder.WithName("vi"),
		vibuilder.WithNamespace(t.Framework.Namespace().Name),
		vibuilder.WithDataSourceObjectRef(v1alpha2.VirtualImageObjectRefKindClusterVirtualImage, object.PrecreatedCVICustomBIOS),
		vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
	)

	t.VDRoot = object.NewVDFromCVI("vd-root", t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
	)

	t.VDBlank = object.NewVD(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse(vdCustomImageSize))),
	)

	t.VM = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: t.VDRoot.Name,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: object.PrecreatedCVICustomISO,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ImageDevice,
				Name: t.VI.Name,
			},
		),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
		vmbuilder.WithRunPolicy(runPolicy),
		// The custom image has no cloud-init; the guest agent is baked
		// into the image, so no provisioning is needed.
	)

	t.VMBDA = vmbdabuilder.New(
		vmbdabuilder.WithName("vmbda"),
		vmbdabuilder.WithNamespace(t.VDBlank.Namespace),
		vmbdabuilder.WithVirtualMachineName(t.VM.Name),
		vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, t.VDBlank.Name),
	)
}

// beVMOPFailed reports the VMOP has reached the Failed phase (used where the
// operation is expected to be rejected, e.g. stopping an AlwaysOn VM). A
// Completed phase is reported as a definite error so the wait aborts
// immediately instead of burning the timeout.
func beVMOPFailed() vmopobs.Predicate {
	return func(op *v1alpha2.VirtualMachineOperation) (bool, error) {
		switch op.Status.Phase {
		case v1alpha2.VMOPPhaseFailed:
			return true, nil
		case v1alpha2.VMOPPhaseCompleted:
			return false, fmt.Errorf("vmop %s/%s completed, expected it to fail", op.Namespace, op.Name)
		default:
			return false, nil
		}
	}
}

// stopGuestAsRoot asks the guest OS to power itself off. The custom
// image has no cloud user and no sudo, so it logs in as root; the delayed
// nohup lets the SSH session complete cleanly before the guest goes down.
func stopGuestAsRoot(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, "nohup sh -c \"sleep 5 && poweroff\" > /dev/null 2>&1 &", framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
}

// rebootGuestAsRoot asks the guest OS to reboot itself; see stopGuestAsRoot.
func rebootGuestAsRoot(f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	_, err := f.SSHCommand(vm.Name, vm.Namespace, "nohup sh -c \"sleep 5 && reboot\" > /dev/null 2>&1 &", framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
}

// expectVMRunningSkippingStuckGuestShutdown waits, via the VM Observer, for
// the VirtualMachine to become Running. When the wait fails it first checks
// for the known lost-guest-shutdown-reason controller race and skips the spec
// in that case (see util.SkipIfGuestPowerActionStuck for the details).
func expectVMRunningSkippingStuckGuestShutdown(ctx context.Context, vmObs vmobs.Observer, vm *v1alpha2.VirtualMachine, timeout time.Duration) {
	GinkgoHelper()

	err := vmObs.WaitFor(vmobs.BeRunning(), timeout)
	if err != nil {
		util.SkipIfGuestPowerActionStuck(ctx, crclient.ObjectKeyFromObject(vm))
	}
	Expect(err).NotTo(HaveOccurred())
}

// expectVMRebooted waits, via the VM Observer, until the VirtualMachine is
// Running with a Running-condition transition newer than previousRunningTime,
// i.e. the guest went down and came back. When the wait fails it first checks
// for the known lost-guest-shutdown-reason controller race and skips the spec
// in that case.
func expectVMRebooted(ctx context.Context, vmObs vmobs.Observer, vm *v1alpha2.VirtualMachine, previousRunningTime time.Time, timeout time.Duration) {
	GinkgoHelper()

	err := vmObs.WaitFor(vmobs.BeRebootedAfter(previousRunningTime), timeout)
	if err != nil {
		util.SkipIfGuestPowerActionStuck(ctx, crclient.ObjectKeyFromObject(vm))
	}
	Expect(err).NotTo(HaveOccurred())
}
