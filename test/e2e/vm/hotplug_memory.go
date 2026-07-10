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
	"os"
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
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

func decoratorsForMemoryHotplugWithLiveMigration() []interface{} {
	if os.Getenv("PARALLEL_MEMORY_HOTPLUG_MIGRATIONS") != "true" {
		return nil
	}
	return []interface{}{Ordered, ContinueOnFailure}
}

var _ = Describe("HotplugMemory", func() {
	var (
		f *framework.Framework
		t *memoryHotplugTest
	)

	BeforeEach(func() {
		f = framework.NewFramework("memory-hotplug")
		DeferCleanup(f.After)
		f.Before()
		t = newMemoryHotplugTest(f)
	})

	Describe("InPlaceResize", Label(precheck.HotplugInPlaceResizePrecheck), func() {
		DescribeTable("should apply memory changes in-place without restart",
			func(initialMemory, changedMemory string) {
				t.applyMemoryChangeInPlace(initialMemory, changedMemory)
			},
			Entry("change memory from 1Gi to 2Gi", "1Gi", "2Gi"),
			Entry("change memory from 1Gi to 4Gi", "1Gi", "4Gi"),
		)

		DescribeTable("should require restart to decrease memory",
			func(initialMemory, changedMemory string) {
				t.requireRestartToDecreaseMemory(initialMemory, changedMemory, false)
			},
			Entry("decrease memory from 2Gi to 1Gi", "2Gi", "1Gi"),
			Entry("decrease memory from 4Gi to 1Gi", "4Gi", "1Gi"),
		)
	})

	Describe("LiveMigration", decoratorsForMemoryHotplugWithLiveMigration(), Label(precheck.HotplugMemoryWithLiveMigrationPrecheck), func() {
		DescribeTable("should apply memory changes via live migration without restart",
			func(initialMemory, changedMemory string) {
				t.applyMemoryChangeWithLiveMigration(initialMemory, changedMemory)
			},
			Entry("change memory from 1Gi to 2Gi", "1Gi", "2Gi"),
			Entry("change memory from 1Gi to 4Gi", "1Gi", "4Gi"),
		)

		DescribeTable("should require restart to decrease memory",
			func(initialMemory, changedMemory string) {
				t.requireRestartToDecreaseMemory(initialMemory, changedMemory, true)
			},
			Entry("decrease memory from 2Gi to 1Gi", "2Gi", "1Gi"),
			Entry("decrease memory from 4Gi to 1Gi", "4Gi", "1Gi"),
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

	By("Wait until VM agent is ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

	By("Waiting for VM agent to be ready")
	util.UntilSSHReady(t.Framework, t.VM, framework.ShortTimeout)

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
	Expect(util.IsRestartRequired(t.VM, framework.ShortTimeout)).To(BeTrue())
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

	By("Wait until VM agent is ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

	By("Waiting for VM agent to be ready")
	util.UntilSSHReady(t.Framework, t.VM, framework.ShortTimeout)

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
		util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VM), framework.MaxTimeout)
	} else {
		By("Waiting until memory size is applied in-place")
		untilVMMemorySizeApplied(crclient.ObjectKeyFromObject(t.VM), changedQuantity, framework.MaxTimeout)
		util.ExpectNoVMOperationsForVirtualMachine(ctx, t.Framework, t.VM)
		util.ExpectVMOnNode(ctx, t.Framework, t.VM, initialNode)
	}

	util.UntilSSHReady(t.Framework, t.VM, framework.MiddleTimeout)

	By("Checking changed memory size")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.Memory.Size).To(Equal(changedQuantity))

	guestMemorySize, err = t.getGuestMemorySize()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestMemorySize).To(Equal(int(changedQuantity.Value())))
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
	t.VD = object.NewVDFromCVI(vdName, t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse("400Mi"))),
	)

	opts := []vmbuilder.Option{
		vmbuilder.WithName(vmName),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(2, ptr.To("10%")),
		vmbuilder.WithMemory(memSizeQuantity),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
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

var totalOnlineMemRe = regexp.MustCompile(`^Total online memory:\s+(\d+)$`)

func (t *memoryHotplugTest) getGuestMemorySize() (int, error) {
	cmdOut, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, "lsmem -b --summary=only")
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

func untilVMMemorySizeApplied(key crclient.ObjectKey, expectedSize resource.Quantity, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm, err := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vm.Status.Resources.Memory.Size).To(Equal(expectedSize))
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}
