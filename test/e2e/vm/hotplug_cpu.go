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
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const disableInPlaceResizeAnn = "kubevirt.internal.virtualization.deckhouse.io/disable-in-place-resize"

func decoratorsForCPUHotplugWithLiveMigration() []interface{} {
	if os.Getenv("PARALLEL_CPU_HOTPLUG_MIGRATIONS") != "true" {
		return nil
	}
	return []interface{}{Ordered, ContinueOnFailure}
}

var _ = Describe("HotplugCPU", func() {
	var (
		f *framework.Framework
		t *cpuHotplugTest
	)

	BeforeEach(func() {
		f = framework.NewFramework("cpu-hotplug")
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

	Describe("LiveMigration", decoratorsForCPUHotplugWithLiveMigration(), Label(precheck.HotplugCPUWithLiveMigrationPrecheck), func() {
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

	By("Wait until VM agent is ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

	By("Waiting for VM agent to be ready")
	util.UntilSSHReady(t.Framework, t.VM, framework.ShortTimeout)

	By("Checking initial CPU configuration")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(initialCores))

	guestCPUCount, err := t.getGuestCPUCount()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestCPUCount).To(Equal(initialCores))

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
	util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhasePending), framework.LongTimeout, vmop)

	By("Checking CPU configuration is not applied before migration can proceed")
	guestCPUCount, err = t.getGuestCPUCount()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestCPUCount).To(Equal(initialCores))

	By("Removing resource quota")
	err = t.Framework.GenericClient().Delete(ctx, quota)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting until CPU configuration is applied via live migration")
	util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VM), framework.MaxTimeout)
	util.UntilObjectPhase(ctx, string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, vmop)

	util.UntilSSHReady(t.Framework, t.VM, framework.MiddleTimeout)

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

	By("Wait until VM agent is ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

	By("Waiting for VM agent to be ready")
	util.UntilSSHReady(t.Framework, t.VM, framework.ShortTimeout)

	By("Checking initial CPU configuration")
	err = t.Framework.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(initialCores))

	guestCPUCount, err := t.getGuestCPUCount()
	Expect(err).NotTo(HaveOccurred())
	Expect(guestCPUCount).To(Equal(initialCores))

	initialNode, err := util.GetVMNode(ctx, t.Framework, t.VM)
	Expect(err).NotTo(HaveOccurred())

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
		util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VM), framework.MaxTimeout)
	} else {
		By("Waiting until CPU configuration is applied in-place")
		untilVMCPUCoresApplied(crclient.ObjectKeyFromObject(t.VM), changedCores, framework.MaxTimeout)
		util.ExpectNoVMOperationsForVirtualMachine(ctx, t.Framework, t.VM)
		util.ExpectVMOnNode(ctx, t.Framework, t.VM, initialNode)
	}

	util.UntilSSHReady(t.Framework, t.VM, framework.MiddleTimeout)

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
	t.VD = object.NewVDFromCVI(vdName, t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
	)

	opts := []vmbuilder.Option{
		vmbuilder.WithName(vmName),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(cores, ptr.To("10%")),
		vmbuilder.WithMemory(*resource.NewQuantity(object.Mi256, resource.BinarySI)),
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

func (t *cpuHotplugTest) getGuestCPUCount() (int, error) {
	cmdOut, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, "nproc")
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

	Eventually(func(g Gomega) {
		count, err := t.getGuestCPUCount()
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(count).To(Equal(expectedCores))
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func untilVMCPUCoresApplied(key crclient.ObjectKey, expectedCores int, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm, err := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vm.Status.Resources.CPU.Cores).To(Equal(expectedCores))
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func untilHotplugMigrationVMOPCreated(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine, timeout time.Duration) *v1alpha2.VirtualMachineOperation {
	GinkgoHelper()

	var createdVMOP *v1alpha2.VirtualMachineOperation

	Eventually(func(g Gomega) {
		vmops, err := f.VirtClient().VirtualMachineOperations(vm.Namespace).List(ctx, metav1.ListOptions{})
		g.Expect(err).NotTo(HaveOccurred())

		for i := range vmops.Items {
			vmop := &vmops.Items[i]
			if vmop.Spec.VirtualMachine != vm.Name {
				continue
			}
			if vmop.Spec.Type != v1alpha2.VMOPTypeEvict {
				continue
			}
			if vmop.Annotations[annotations.AnnVMOPWorkloadUpdate] != "true" {
				continue
			}

			createdVMOP = vmop.DeepCopy()
			return
		}

		g.Expect(createdVMOP).NotTo(BeNil())
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())

	return createdVMOP
}
