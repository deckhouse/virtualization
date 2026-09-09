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
	"regexp"
	"slices"
	"strconv"
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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// Memory sizes for the hotplug specs. They are not tied to the custom guest
// image, which needs far less: hotplug is enabled only from 1Gi up
// (kvbuilder.EnableMemoryHotplugThreshold), so that is the floor, and the guest
// onlines memory in 128Mi blocks (lsmem "Memory block size" on x86_64), which the
// specs compare against exactly. One and two blocks above the floor are therefore
// the two smallest distinct steps this suite can exercise.
const (
	hotplugMemoryFloor  = "1Gi"
	memoryPlusOneBlock  = "1152Mi"
	memoryPlusTwoBlocks = "1280Mi"
)

var _ = Describe("HotplugMemory", Label(label.SIGCompute), func() {
	var (
		f *framework.Framework
		t *memoryHotplugTest
	)

	BeforeEach(func() {
		f = framework.NewFramework("hotplug-memory")
		DeferCleanup(f.After)
		f.Before()
		t = newMemoryHotplugTest(f)
	})

	Describe("InPlaceResize", Label(precheck.HotplugInPlaceResizePrecheck), func() {
		DescribeTable("should apply memory changes in-place without restart",
			func(initialMemory, changedMemory string) {
				t.applyMemoryChangeInPlace(initialMemory, changedMemory)
			},
			Entry("change memory from 1Gi to 1152Mi", hotplugMemoryFloor, memoryPlusOneBlock),
			Entry("change memory from 1Gi to 1280Mi", hotplugMemoryFloor, memoryPlusTwoBlocks),
		)

		DescribeTable("should require restart to decrease memory",
			func(initialMemory, changedMemory string) {
				t.requireRestartToDecreaseMemory(initialMemory, changedMemory, false)
			},
			Entry("decrease memory from 1152Mi to 1Gi", memoryPlusOneBlock, hotplugMemoryFloor),
			Entry("decrease memory from 1280Mi to 1Gi", memoryPlusTwoBlocks, hotplugMemoryFloor),
		)
	})

	Describe("LiveMigration", Label(precheck.HotplugMemoryWithLiveMigrationPrecheck), func() {
		DescribeTable("should apply memory changes via live migration without restart",
			func(initialMemory, changedMemory string) {
				t.applyMemoryChangeWithLiveMigration(initialMemory, changedMemory)
			},
			Entry("change memory from 1Gi to 1152Mi", hotplugMemoryFloor, memoryPlusOneBlock),
			// TODO: Re-enable the entry. Under parallel load the migration
			// intermittently never starts within the wait timeout.
			XEntry("change memory from 1Gi to 1280Mi", hotplugMemoryFloor, memoryPlusTwoBlocks),
		)

		DescribeTable("should require restart to decrease memory",
			func(initialMemory, changedMemory string) {
				t.requireRestartToDecreaseMemory(initialMemory, changedMemory, true)
			},
			Entry("decrease memory from 1152Mi to 1Gi", memoryPlusOneBlock, hotplugMemoryFloor),
			Entry("decrease memory from 1280Mi to 1Gi", memoryPlusTwoBlocks, hotplugMemoryFloor),
		)
	})
})

type memoryHotplugTest struct {
	Framework *framework.Framework

	VM *v1alpha2.VirtualMachine
	VD *v1alpha2.VirtualDisk
}

func newMemoryHotplugTest(f *framework.Framework) *memoryHotplugTest {
	return &memoryHotplugTest{Framework: f}
}

func (t *memoryHotplugTest) applyMemoryChangeInPlace(initialMemory, changedMemory string) {
	t.applyMemoryChange(initialMemory, changedMemory, false)
}

func (t *memoryHotplugTest) applyMemoryChangeWithLiveMigration(initialMemory, changedMemory string) {
	t.applyMemoryChange(initialMemory, changedMemory, true)
}

func (t *memoryHotplugTest) requireRestartToDecreaseMemory(initialMemory, changedMemory string, liveMigration bool) {
	ctx := context.Background()
	initialQuantity := resource.MustParse(initialMemory)

	By("Environment preparation")
	vmName := strings.ToLower(fmt.Sprintf("vm-%s-%s-decrease", initialMemory, changedMemory))
	if liveMigration {
		vmName += "-migrate"
	}
	t.generateResourcesWithRestartApproval(vmName, initialMemory, liveMigration, v1alpha2.Manual)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VM, t.VD)
	Expect(err).NotTo(HaveOccurred())

	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)
	vmObs.Never(vmobs.BeFailed())

	By("Wait until VM agent is ready")
	err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for VM agent to be ready")
	// LongTimeout: under the parallel run the first SSH banner exchange can
	// take well over ShortTimeout on a loaded node.
	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.LongTimeout)

	initialNode, err := util.GetVMNode(ctx, t.Framework, t.VM)
	Expect(err).NotTo(HaveOccurred())

	initialGuestMemorySize, err := t.getGuestMemorySize()
	Expect(err).NotTo(HaveOccurred())
	Expect(initialGuestMemorySize).To(Equal(int(initialQuantity.Value())))

	By("Applying memory decrease")
	patch, err := json.Marshal([]map[string]interface{}{{
		"op":    "replace",
		"path":  "/spec/memory/size",
		"value": changedMemory,
	}})
	Expect(err).NotTo(HaveOccurred())
	err = t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))
	Expect(err).NotTo(HaveOccurred())

	By("Waiting until restart is required")
	// The VM uses the Manual restart approval mode, so decreasing memory must
	// park it awaiting a restart instead of hotplugging.
	err = vmObs.WaitFor(vmobs.BeAwaitingRestart(), framework.ShortTimeout)
	Expect(err).NotTo(HaveOccurred())
	util.ExpectNoVMOperationsForVirtualMachine(ctx, t.Framework, t.VM)
	util.ExpectVMOnNode(ctx, t.Framework, t.VM, initialNode)

	By("Checking memory size is not applied without restart")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.Memory.Size).To(Equal(initialQuantity))

	guestMemorySize, err := t.getGuestMemorySize()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestMemorySize).To(Equal(initialGuestMemorySize))
}

func (t *memoryHotplugTest) applyMemoryChange(initialMemory, changedMemory string, liveMigration bool) {
	ctx := context.Background()
	initialQuantity := resource.MustParse(initialMemory)
	changedQuantity := resource.MustParse(changedMemory)

	By("Environment preparation")
	vmName := strings.ToLower(fmt.Sprintf("vm-%s-%s", initialMemory, changedMemory))
	if liveMigration {
		vmName += "-migrate"
	}
	t.generateResources(vmName, initialMemory, liveMigration)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VM, t.VD)
	Expect(err).NotTo(HaveOccurred())

	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)
	vmObs.Never(vmobs.BeFailed())

	By("Wait until VM agent is ready")
	err = vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for VM agent to be ready")
	// LongTimeout: under the parallel run the first SSH banner exchange can
	// take well over ShortTimeout on a loaded node.
	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.LongTimeout)

	By("Checking initial memory size")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.Memory.Size).To(Equal(initialQuantity))

	guestMemorySize, err := t.getGuestMemorySize()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestMemorySize).To(Equal(int(initialQuantity.Value())))

	initialNode, err := util.GetVMNode(ctx, t.Framework, t.VM)
	Expect(err).NotTo(HaveOccurred())

	if liveMigration {
		skipIfDisksAreNotLiveMigratable(ctx, t.Framework, t.VD)
	}

	By("Applying memory size changes")
	patch, err := json.Marshal([]map[string]interface{}{{
		"op":    "replace",
		"path":  "/spec/memory/size",
		"value": changedMemory,
	}})
	Expect(err).NotTo(HaveOccurred())
	err = t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))
	Expect(err).NotTo(HaveOccurred())

	if liveMigration {
		By("Waiting until memory size is applied via live migration")
		err = vmObs.WaitFor(vmobs.HaveMigrationSucceeded(), framework.MaxTimeout)
		if err != nil {
			// TODO: remove temporary migration skip logic when both known issues are
			// fixed: kubevirt "client socket is closed" and Volume(s)UpdateError.
			util.SkipIfKnownMigrationFailureWithContext(ctx, t.VM)
		}
		Expect(err).NotTo(HaveOccurred())
	} else {
		By("Waiting until memory size is applied in-place")
		err = vmObs.WaitFor(haveMemorySize(changedQuantity), framework.MaxTimeout)
		Expect(err).NotTo(HaveOccurred())
		// In-place resize needs the node to absorb the launcher pod's enlarged
		// requests: when kubelet defers or rejects the pod resize, the workload
		// updater applies the change through a one-shot live migration (a
		// hotplug-resources-* VMOP) instead — still without a guest restart.
		// The fallback is capacity-driven, so it is tolerated here; the strict
		// in-place expectations hold only when no migration was triggered.
		if t.findHotplugResourcesVMOP(ctx) == nil {
			util.ExpectNoVMOperationsForVirtualMachine(ctx, t.Framework, t.VM)
			util.ExpectVMOnNode(ctx, t.Framework, t.VM, initialNode)
		} else {
			By("Waiting until the fallback hotplug migration succeeds")
			util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VM), framework.MaxTimeout)
		}
	}

	eventually.SSHReadyAsRoot(t.Framework, t.VM, framework.MiddleTimeout)

	By("Checking changed memory size")
	// The migration finishing does not mean the VM status already carries the
	// new memory size: the controller updates status.resources.memory.size a
	// moment later, so a one-shot read here races it.
	err = vmObs.WaitFor(haveMemorySize(changedQuantity), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())

	t.untilGuestMemorySize(int(changedQuantity.Value()), framework.MiddleTimeout)
}

// untilGuestMemorySize waits until the guest reports the expected amount of
// online memory. The platform reporting the new size only means the memory was
// plugged in; the guest still has to bring the added blocks online, and reading
// it once right after the platform settles races that.
func (t *memoryHotplugTest) untilGuestMemorySize(expected int, timeout time.Duration) {
	GinkgoHelper()

	// EXCEPTION: guest-side wait (lsmem over SSH), not a Kubernetes resource —
	// nothing to observe via an Observer.
	eventually.UntilAssertion(func(g Gomega) {
		size, err := t.getGuestMemorySize()
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(size).To(Equal(expected))
	}, timeout)
}

// TODO: Remove this skip when CPU/memory hotplug is supported for VMs with RWO disks.
// Upstream KubeVirt hotplugs CPU and memory only for a plainly live-migratable VMI:
// the hotplug handlers check vmi.IsMigratable() (the LiveMigratable condition)
// and know nothing about the StorageLiveMigratable condition our fork uses to volume-migrate
// VMs with RWO disks. So on an RWO storage class KubeVirt sets RestartRequired instead of
// hotplugging, the VM parks awaiting a restart (the workload-updater never creates the
// migration VMOP), and the migration these tests wait for never happens.
func skipIfDisksAreNotLiveMigratable(ctx context.Context, f *framework.Framework, vdRef *v1alpha2.VirtualDisk) {
	GinkgoHelper()

	vd := &v1alpha2.VirtualDisk{}
	err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vdRef), vd)
	Expect(err).NotTo(HaveOccurred())

	pvc, err := f.KubeClient().CoreV1().PersistentVolumeClaims(vd.Namespace).Get(ctx, vd.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	if slices.Contains(pvc.Spec.AccessModes, corev1.ReadWriteMany) {
		return
	}

	Skip(fmt.Sprintf("skip: PVC %s/%s is not ReadWriteMany, hotplug via live migration needs a live-migratable VMI", pvc.Namespace, pvc.Name))
}

func (t *memoryHotplugTest) generateResources(vmName, memSize string, disableInPlaceResize bool) {
	t.generateResourcesWithRestartApproval(vmName, memSize, disableInPlaceResize, v1alpha2.Automatic)
}

func (t *memoryHotplugTest) generateResourcesWithRestartApproval(vmName, memSize string, disableInPlaceResize bool, restartApprovalMode v1alpha2.RestartApprovalMode) {
	memSizeQuantity := resource.MustParse(memSize)

	vdName := fmt.Sprintf("vd-%s-root", vmName)
	t.VD = object.NewVDFromCVI(vdName, t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
	)

	opts := []vmbuilder.Option{
		vmbuilder.WithName(vmName),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		// The CPU is incidental here: memory is what this suite exercises.
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(memSizeQuantity),
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

// findHotplugResourcesVMOP returns the workload updater's one-shot migration
// VMOP for the VM (created when an in-place resize is not feasible on the
// current node), or nil if the resize went in-place.
func (t *memoryHotplugTest) findHotplugResourcesVMOP(ctx context.Context) *v1alpha2.VirtualMachineOperation {
	vmops, err := t.Framework.VirtClient().VirtualMachineOperations(t.VM.Namespace).List(ctx, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	for i := range vmops.Items {
		vmop := &vmops.Items[i]
		if vmop.Spec.VirtualMachine == t.VM.Name && strings.HasPrefix(vmop.Name, "hotplug-resources-") {
			return vmop
		}
	}
	return nil
}

var totalOnlineMemRe = regexp.MustCompile(`^Total online memory:\s+(\d+)$`)

func (t *memoryHotplugTest) getGuestMemorySize() (int, error) {
	cmdOut, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, "lsmem -b --summary=only", framework.WithSSHUser("root"))
	if err != nil {
		return 0, err
	}

	lines := strings.Split(cmdOut, "\n")

	for _, line := range lines {
		matches := totalOnlineMemRe.FindStringSubmatch(line)
		if len(matches) >= 2 {
			return strconv.Atoi(matches[1])
		}
	}

	return 0, fmt.Errorf("failed to find total online memory in lsmem output: %v", cmdOut)
}

// haveMemorySize reports the VM status carries exactly the given memory size.
func haveMemorySize(size resource.Quantity) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		return vm.Status.Resources.Memory.Size.Equal(size), nil
	}
}
