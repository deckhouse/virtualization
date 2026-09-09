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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const disableInPlaceResizeAnn = "kubevirt.internal.virtualization.deckhouse.io/disable-in-place-resize"

var _ = Describe("HotplugCPU", Label(label.SIGCompute), func() {
	var (
		f *framework.Framework
		t *cpuHotplugTest
	)

	BeforeEach(func() {
		// TODO: Re-enable the suite once the workload-updater no longer races with
		// virt-handler on in-place resize completion: the in-place-resize-in-progress
		// annotation is removed before the VCPUChange condition is cleared, so the
		// HotplugHandler sees a plain CPU hotplug and creates a spurious
		// hotplug-resources migration VMOP.
		Skip("temporarily skipped on this branch")

		f = framework.NewFramework("hotplug-cpu")
		DeferCleanup(f.After)
		f.Before()
		t = newCPUHotplugTest(f)
	})

	Describe("InPlaceResize", Label(precheck.HotplugInPlaceResizePrecheck), func() {
		DescribeTable("should apply cpu core changes in-place without restart",
			func(initialCores, changedCores int) {
				t.applyCPUCoreChangeInPlace(initialCores, changedCores)
			},
			Entry("one socket topology, change cores from 1 to 2", 1, 2),
			Entry("one socket topology, change cores from 1 to 4", 1, 4),
			Entry("one socket topology, change cores from 4 to 3", 4, 3),
		)
	})

	Describe("LiveMigration", Label(precheck.HotplugCPUWithLiveMigrationPrecheck), func() {
		DescribeTable("should apply cpu core changes via live migration without restart",
			func(initialCores, changedCores int) {
				t.applyCPUCoreChangeWithLiveMigration(initialCores, changedCores)
			},
			Entry("one socket topology, change cores from 1 to 2", 1, 2),
			Entry("one socket topology, change cores from 1 to 4", 1, 4),
			Entry("one socket topology, change cores from 4 to 3", 4, 3),
		)
	})

	Describe("QuotaBlockedMigration",
		Label(precheck.HotplugInPlaceResizePrecheck),
		Label(precheck.HotplugCPUWithLiveMigrationPrecheck), func() {
			It("should wait for quota removal and then migrate to apply cpu hotplug", func() {
				t.applyCPUCoreChangeWithQuotaBlockedMigration(1, 4, resource.MustParse("2"))
			})
		})
})

type cpuHotplugTest struct {
	Framework *framework.Framework

	VM *v1alpha2.VirtualMachine
	VD *v1alpha2.VirtualDisk
}

func newCPUHotplugTest(f *framework.Framework) *cpuHotplugTest {
	return &cpuHotplugTest{Framework: f}
}

func (t *cpuHotplugTest) applyCPUCoreChangeInPlace(initialCores, changedCores int) {
	t.applyCPUCoreChange(initialCores, changedCores, false)
}

func (t *cpuHotplugTest) applyCPUCoreChangeWithLiveMigration(initialCores, changedCores int) {
	t.applyCPUCoreChange(initialCores, changedCores, true)
}

func (t *cpuHotplugTest) applyCPUCoreChangeWithQuotaBlockedMigration(initialCores, changedCores int, cpuLimitQuota resource.Quantity) {
	ctx := context.Background()

	By("Environment preparation")
	vmName := fmt.Sprintf("vm-%d-%d-quota-migrate", initialCores, changedCores)
	t.generateResources(vmName, initialCores, true)

	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "project-quota",
			Namespace: t.Framework.Namespace().Name,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceLimitsCPU: cpuLimitQuota,
			},
		},
	}

	err := t.Framework.CreateWithDeferredDeletion(ctx, quota, t.VM, t.VD)
	Expect(err).NotTo(HaveOccurred())

	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)
	vmObs.Never(vmobs.BeFailed())

	By("Wait until VM agent is ready")
	err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for VM agent to be ready")
	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.ShortTimeout)

	By("Checking initial CPU configuration")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(initialCores))

	guestCPUCount, err := t.getGuestCPUCount()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestCPUCount).To(Equal(initialCores))

	skipIfDisksAreNotLiveMigratable(ctx, t.Framework, t.VD)

	By("Applying CPU core changes")
	patch, err := json.Marshal([]map[string]interface{}{{
		"op":    "replace",
		"path":  "/spec/cpu/cores",
		"value": changedCores,
	}})
	Expect(err).NotTo(HaveOccurred())
	err = t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for workload updater to create migration VMOP")
	vmop := untilHotplugMigrationVMOPCreated(ctx, t.Framework, t.VM, framework.MaxTimeout)
	// The discovery above already saw the VMOP parked in Pending by the quota;
	// the observer is armed now so its later transitions are captured.
	vmopObs := vmopobs.StartObserver(ctx, vmop)

	By("Checking CPU configuration is not applied before migration can proceed")
	guestCPUCount, err = t.getGuestCPUCount()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestCPUCount).To(Equal(initialCores))

	By("Removing resource quota")
	err = t.Framework.GenericClient().Delete(ctx, quota)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting until CPU configuration is applied via live migration")
	err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MaxTimeout)
	if err != nil {
		// TODO: remove temporary migration skip logic when both known issues are
		// fixed: kubevirt "client socket is closed" and Volume(s)UpdateError.
		util.SkipIfKnownMigrationFailureWithContext(ctx, t.VM)
	}
	Expect(err).NotTo(HaveOccurred())
	err = vmopObs.WaitFor(vmopobs.BeCompleted(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.MiddleTimeout)

	By("Checking changed CPU configuration")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(changedCores))

	t.untilGuestCPUCount(changedCores, framework.MiddleTimeout)
}

func (t *cpuHotplugTest) applyCPUCoreChange(initialCores, changedCores int, liveMigration bool) {
	ctx := context.Background()

	By("Environment preparation")
	vmName := fmt.Sprintf("vm-%d-%d", initialCores, changedCores)
	if liveMigration {
		vmName += "-migrate"
	}
	t.generateResources(vmName, initialCores, liveMigration)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VM, t.VD)
	Expect(err).NotTo(HaveOccurred())

	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)
	vmObs.Never(vmobs.BeFailed())

	By("Wait until VM agent is ready")
	err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for VM agent to be ready")
	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.ShortTimeout)

	By("Checking initial CPU configuration")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(initialCores))

	guestCPUCount, err := t.getGuestCPUCount()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestCPUCount).To(Equal(initialCores))

	initialNode, err := util.GetVMNode(ctx, t.Framework, t.VM)
	Expect(err).NotTo(HaveOccurred())

	if liveMigration {
		skipIfDisksAreNotLiveMigratable(ctx, t.Framework, t.VD)
	}

	By("Applying CPU core changes")
	patch, err := json.Marshal([]map[string]interface{}{{
		"op":    "replace",
		"path":  "/spec/cpu/cores",
		"value": changedCores,
	}})
	Expect(err).NotTo(HaveOccurred())
	err = t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))
	Expect(err).NotTo(HaveOccurred())

	if liveMigration {
		By("Waiting until CPU configuration is applied via live migration")
		err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MaxTimeout)
		if err != nil {
			// TODO: remove temporary migration skip logic when both known issues are
			// fixed: kubevirt "client socket is closed" and Volume(s)UpdateError.
			util.SkipIfKnownMigrationFailureWithContext(ctx, t.VM)
		}
		Expect(err).NotTo(HaveOccurred())
	} else {
		By("Waiting until CPU configuration is applied in-place")
		err = vmObs.WaitFor(haveCPUCores(changedCores), framework.MaxTimeout)
		Expect(err).NotTo(HaveOccurred())
		util.ExpectNoVMOperationsForVirtualMachine(ctx, t.Framework, t.VM)
		util.ExpectVMOnNode(ctx, t.Framework, t.VM, initialNode)
	}

	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.MiddleTimeout)

	By("Checking changed CPU configuration")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(changedCores))

	t.untilGuestCPUCount(changedCores, framework.MiddleTimeout)
}

func (t *cpuHotplugTest) generateResources(vmName string, cores int, disableInPlaceResize bool) {
	t.generateResourcesWithRestartApproval(vmName, cores, disableInPlaceResize, v1alpha2.Automatic)
}

func (t *cpuHotplugTest) generateResourcesWithRestartApproval(vmName string, cores int, disableInPlaceResize bool, restartApprovalMode v1alpha2.RestartApprovalMode) {
	vdName := fmt.Sprintf("vd-%s-root", vmName)
	t.VD = object.NewVDFromCVI(vdName, t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
	)

	opts := []vmbuilder.Option{
		vmbuilder.WithName(vmName),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(cores, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(*resource.NewQuantity(object.Mi64, resource.BinarySI)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		// The custom image has no cloud-init: the guest agent is baked in
		// and hotplugged CPUs/memory are onlined by the image itself (the
		// /sbin/hotplug uevent helper and the kernel-side default-online), which
		// replaces the udev rules cloud-init installed on the Alpine image.
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: t.VD.Name,
			},
		),
		vmbuilder.WithRestartApprovalMode(restartApprovalMode),
	}
	if disableInPlaceResize {
		opts = append(opts, vmbuilder.WithAnnotation(disableInPlaceResizeAnn, "true"))
	}

	t.VM = vmbuilder.New(opts...)
}

func (t *cpuHotplugTest) getGuestCPUCount() (int, error) {
	cmdOut, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, "nproc", framework.WithSSHUser("root"))
	if err != nil {
		return 0, err
	}

	var cpuCount int
	_, err = fmt.Sscanf(strings.TrimSpace(cmdOut), "%d", &cpuCount)
	if err != nil {
		return 0, fmt.Errorf("parse guest cpu count from %q: %w", cmdOut, err)
	}

	return cpuCount, nil
}

func (t *cpuHotplugTest) untilGuestCPUCount(expectedCores int, timeout time.Duration) {
	GinkgoHelper()

	// EXCEPTION: guest-side wait (nproc over SSH), not a Kubernetes resource —
	// nothing to observe via an Observer.
	eventually.UntilAssertion(func(g Gomega) {
		count, err := t.getGuestCPUCount()
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(count).To(Equal(expectedCores))
	}, timeout)
}

// untilHotplugMigrationVMOPCreated waits for the workload updater's migration
// VMOP for vm and returns it once it is parked in the Pending phase. The VMOP
// is created asynchronously with a generated name the test cannot know in
// advance, so it is discovered by watching the namespace's VMOPs.
func untilHotplugMigrationVMOPCreated(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, timeout time.Duration) *v1alpha2.VirtualMachineOperation {
	GinkgoHelper()

	vmop, err := observer.WaitForFirst(ctx,
		f.VirtClient().VirtualMachineOperations(vm.Namespace),
		timeout,
		func(vmop *v1alpha2.VirtualMachineOperation) bool {
			return vmop.Spec.VirtualMachine == vm.Name &&
				vmop.Spec.Type == v1alpha2.VMOPTypeEvict &&
				vmop.Annotations[annotations.AnnVMOPWorkloadUpdate] == "true" &&
				vmop.Status.Phase == v1alpha2.VMOPPhasePending
		})
	Expect(err).NotTo(HaveOccurred(),
		"no pending workload-update migration vmop found for vm %s/%s", vm.Namespace, vm.Name)

	return vmop
}

// haveCPUCores reports the VM status carries exactly the given core count.
func haveCPUCores(cores int) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		return vm.Status.Resources.CPU.Cores == cores, nil
	}
}
