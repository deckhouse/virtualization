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
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("CoreFraction", func() {
	var (
		f *framework.Framework
		t *coreFractionTest
	)

	BeforeEach(func() {
		f = framework.NewFramework("core-fraction")
		DeferCleanup(f.After)
		f.Before()
		t = &coreFractionTest{Framework: f}
	})

	Context("GeneralCoreFraction", Label(precheck.HotplugInPlaceResizePrecheck), func() {
		It("should apply an explicit coreFraction change in-place and update pod CPU requests", func() {
			t.applyExplicitCoreFractionChange(1, "5%", "10%")
		})
	})

	Context("AutoCoreFraction", Label(precheck.HotplugInPlaceResizePrecheck), Label(precheck.PrecheckVerticalPodAutoscaler), func() {
		It("should autoscale coreFraction from a pinned VPA recommendation", func() {
			t.autoscaleCoreFractionViaRecommendation(1)
		})
	})

	// In-place resize is switched off on the instance, so every change can only be applied by a
	// live migration, as on a node without free CPU, while no node has to be filled up.
	Context("LiveMigration", Label(precheck.HotplugInPlaceResizePrecheck), Label(precheck.HotplugCPUWithLiveMigrationPrecheck), func() {
		It("should migrate every coreFraction change, including a return to a value migrated before", func() {
			t.migrateCoreFractionChanges(1, "5%", "10%", "5%", "10%")
		})
	})

	Context("AutoCoreFractionLiveMigration", Label(precheck.HotplugInPlaceResizePrecheck), Label(precheck.HotplugCPUWithLiveMigrationPrecheck), Label(precheck.PrecheckVerticalPodAutoscaler), func() {
		It("should migrate every autoscaler step that is not applied in place", func() {
			t.migrateAutoscalerSteps(1)
		})
	})

	Context("AutoCoreFractionNonMigratable", Label(precheck.HotplugInPlaceResizePrecheck), Label(precheck.PrecheckVerticalPodAutoscaler), func() {
		It("should apply an autoscaler step in place without asking for a restart", func() {
			t.autoscaleNonMigratable(1)
		})
	})

	Context("ReturnAfterInPlaceChange", Label(precheck.HotplugInPlaceResizePrecheck), Label(precheck.HotplugCPUWithLiveMigrationPrecheck), func() {
		It("should migrate a return to the coreFraction the launcher pod was migrated for, after a change applied in place", func() {
			t.returnAfterInPlaceChange(1, "5%", "10%")
		})
	})

	Context("ChangeWhileAnotherIsPending", Label(precheck.HotplugInPlaceResizePrecheck), Label(precheck.HotplugCPUWithLiveMigrationPrecheck), func() {
		It("should apply a coreFraction change made while the previous one waits for its live migration", func() {
			t.changeCoreFractionWhilePending(1, "5%", "10%", "20%")
		})
	})
})

type coreFractionTest struct {
	Framework *framework.Framework

	VM *v1alpha2.VirtualMachine
	VD *v1alpha2.VirtualDisk
}

func (t *coreFractionTest) applyExplicitCoreFractionChange(cores int, initial, changed string) {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-core-fraction", cores, initial)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())
	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)

	By("Waiting for VM agent to be ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

	initialNode, err := util.GetVMNode(ctx, t.Framework, t.VM)
	Expect(err).NotTo(HaveOccurred())

	By("Checking the initial pod CPU request matches the initial coreFraction")
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, initial), framework.ShortTimeout)

	By("Changing coreFraction")
	patch, err := json.Marshal([]map[string]interface{}{{
		"op":    "replace",
		"path":  "/spec/cpu/coreFraction",
		"value": changed,
	}})
	Expect(err).NotTo(HaveOccurred())
	err = t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))
	Expect(err).NotTo(HaveOccurred())

	By("Waiting until the change is applied in-place without a restart")
	err = vmObs.WaitFor(haveAppliedCoreFraction(changed), framework.MiddleTimeout)
	Expect(err).NotTo(HaveOccurred())
	util.ExpectNoVMOperationsForVirtualMachine(ctx, t.Framework, t.VM)
	util.ExpectVMOnNode(ctx, t.Framework, t.VM, initialNode)

	By("Checking the pod CPU request follows the new coreFraction")
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, changed), framework.MiddleTimeout)
}

func (t *coreFractionTest) autoscaleCoreFractionViaRecommendation(cores int) {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-core-fraction-auto", cores, v1alpha2.CoreFractionAuto)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())
	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)

	By("Waiting for VM agent to be ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

	By("Reading the seeded coreFraction")
	err = t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	initialFraction := sizingpolicy.RecommendedCoreFraction(t.VM)
	Expect(initialFraction).NotTo(BeEmpty(), "autoscaler should seed status.recommendedResources.cpu.coreFraction")

	By("Waiting until the autoscaler creates the VPA")
	t.untilVPAExists(ctx, framework.MiddleTimeout)

	By("Pinning a high CPU recommendation via the override annotation on the internal VM")
	// Bypass the vpa-recommender's slow, decaying CPU histogram: with a lowerBound above
	// the VM's current CPU request the autoscaler must raise coreFraction, and with a
	// target far beyond capacity the recommended value snaps to the largest Burstable step
	// the sizing policy allows. This exercises the recommendation-to-hotplug seam without
	// any in-guest load.
	t.patchKVVMRecommendationOverride(ctx, recommendationOverrideCPU("20000m", "20000m"))

	By("Waiting until the autoscaler raises coreFraction from the recommendation")
	// The predicate captures the applied value from the very observation that
	// satisfied it, so the pod check below uses the same coreFraction the wait
	// saw.
	var applied string
	err = vmObs.WaitFor(haveRaisedAndAppliedCoreFraction(initialFraction, &applied), framework.MiddleTimeout)
	Expect(err).NotTo(HaveOccurred(), "recommended coreFraction should grow from the pinned recommendation and be applied")

	By("Checking the pod CPU request follows the applied coreFraction")
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, applied), framework.MiddleTimeout)
}

func (t *coreFractionTest) migrateCoreFractionChanges(cores int, initial string, changes ...string) {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-core-fraction-migration", cores, initial)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for VM agent to be ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)
	skipIfDisksAreNotLiveMigratable(ctx, t.Framework, t.VD)

	By("Disabling in-place resize so every change goes through a live migration")
	disableInPlaceResizeOnKVVMI(ctx, t.Framework, t.VM)

	for i, fraction := range changes {
		By(fmt.Sprintf("Changing coreFraction to %s", fraction))
		patch, err := json.Marshal([]map[string]interface{}{{
			"op":    "replace",
			"path":  "/spec/cpu/coreFraction",
			"value": fraction,
		}})
		Expect(err).NotTo(HaveOccurred())
		err = t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Waiting until %s is applied by live migration %d", fraction, i+1))
		t.untilHotplugMigrationsCompleted(ctx, i+1, framework.MaxTimeout)
		t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, fraction), framework.MiddleTimeout)
	}
}

func (t *coreFractionTest) migrateAutoscalerSteps(cores int) {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-core-fraction-auto-migration", cores, v1alpha2.CoreFractionAuto)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())
	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)

	By("Waiting for VM agent to be ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)
	skipIfDisksAreNotLiveMigratable(ctx, t.Framework, t.VD)

	By("Reading the seeded coreFraction")
	err = t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	seeded := sizingpolicy.RecommendedCoreFraction(t.VM)
	Expect(seeded).NotTo(BeEmpty(), "autoscaler should seed status.recommendedResources.cpu.coreFraction")

	By("Waiting until the autoscaler creates the VPA")
	t.untilVPAExists(ctx, framework.MiddleTimeout)

	By("Disabling in-place resize so every step goes through a live migration")
	disableInPlaceResizeOnKVVMI(ctx, t.Framework, t.VM)

	By("Pinning a recommendation just above the seeded coreFraction")
	// The lower bound above the current request makes the autoscaler move, and a target one
	// percent above the seed is rounded up to the next step of the sizing policy.
	next := fmt.Sprintf("%dm", expectedCPURequestMilli(cores, fmt.Sprintf("%d%%", percent(seeded)+1)))
	t.patchKVVMRecommendationOverride(ctx, recommendationOverrideCPU(next, next))

	var first string
	err = vmObs.WaitFor(haveRecommendedAbove(seeded, &first), framework.MiddleTimeout)
	Expect(err).NotTo(HaveOccurred(), "the autoscaler should select the step above %s", seeded)

	By("Waiting until the first step is applied by live migration")
	t.untilHotplugMigrationsCompleted(ctx, 1, framework.MaxTimeout)
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, first), framework.MiddleTimeout)

	By("Pinning a recommendation beyond the capacity of the virtual machine")
	t.patchKVVMRecommendationOverride(ctx, recommendationOverrideCPU("20000m", "20000m"))

	var second string
	err = vmObs.WaitFor(haveRecommendedAbove(first, &second), framework.MiddleTimeout)
	if err != nil {
		Expect(t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)).To(Succeed())
		if sizingpolicy.RecommendedCoreFraction(t.VM) == first {
			Skip(fmt.Sprintf("the sizing policy leaves no step above %s", first))
		}
	}
	Expect(err).NotTo(HaveOccurred(), "the autoscaler should select a step above %s", first)

	By("Waiting until the second step is applied by live migration too")
	t.untilHotplugMigrationsCompleted(ctx, 2, framework.MaxTimeout)
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, second), framework.MiddleTimeout)
}

func (t *coreFractionTest) autoscaleNonMigratable(cores int) {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-core-fraction-auto-pinned", cores, v1alpha2.CoreFractionAuto)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())
	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)

	By("Waiting for VM agent to be ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

	initialNode, err := util.GetVMNode(ctx, t.Framework, t.VM)
	Expect(err).NotTo(HaveOccurred())

	By("Pinning the VM to its node so that it cannot be migrated")
	patch, err := json.Marshal([]map[string]interface{}{{
		"op":    "add",
		"path":  "/spec/nodeSelector",
		"value": map[string]string{corev1.LabelHostname: initialNode},
	}})
	Expect(err).NotTo(HaveOccurred())
	err = t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))
	Expect(err).NotTo(HaveOccurred())
	err = vmObs.WaitFor(beNonMigratable(), framework.MiddleTimeout)
	Expect(err).NotTo(HaveOccurred(), "a VM pinned to its node should become non-migratable")

	By("Reading the seeded coreFraction")
	err = t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	seeded := sizingpolicy.RecommendedCoreFraction(t.VM)
	Expect(seeded).NotTo(BeEmpty(), "autoscaler should seed status.recommendedResources.cpu.coreFraction")

	By("Waiting until the autoscaler creates the VPA")
	t.untilVPAExists(ctx, framework.MiddleTimeout)

	By("Pinning a high CPU recommendation via the override annotation on the internal VM")
	t.patchKVVMRecommendationOverride(ctx, recommendationOverrideCPU("20000m", "20000m"))

	By("Waiting until the step is applied in place")
	var applied string
	err = vmObs.WaitFor(haveRaisedAndAppliedCoreFraction(seeded, &applied), framework.MiddleTimeout)
	Expect(err).NotTo(HaveOccurred(), "the autoscaler step above %s should be applied", seeded)
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, applied), framework.MiddleTimeout)

	Expect(t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)).To(Succeed())
	Expect(t.VM.Status.RestartAwaitingChanges).To(BeEmpty(), "an autoscaler step should never ask for a restart")
	util.ExpectNoVMOperationsForVirtualMachine(ctx, t.Framework, t.VM)
	util.ExpectVMOnNode(ctx, t.Framework, t.VM, initialNode)
}

func (t *coreFractionTest) changeCoreFractionWhilePending(cores int, initial, pending, latest string) {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-core-fraction-pending", cores, initial)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for VM agent to be ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)
	skipIfDisksAreNotLiveMigratable(ctx, t.Framework, t.VD)

	By("Disabling in-place resize so the change waits for a live migration")
	disableInPlaceResizeOnKVVMI(ctx, t.Framework, t.VM)

	By(fmt.Sprintf("Changing coreFraction to %s", pending))
	t.patchCoreFraction(ctx, pending)

	By("Waiting until the live migration for the change is created")
	t.untilHotplugMigrationsCreated(ctx, 1, framework.MiddleTimeout)

	By(fmt.Sprintf("Changing coreFraction to %s while %s is still pending", latest, pending))
	// KubeVirt rejects a CPU change on top of a pending one, so the newer value has to wait
	// for the migration in flight and get a migration of its own.
	t.patchCoreFraction(ctx, latest)

	By(fmt.Sprintf("Waiting until %s is applied by a second live migration", latest))
	t.untilHotplugMigrationsCompleted(ctx, 2, framework.MaxTimeout)
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, latest), framework.MiddleTimeout)
}

func (t *coreFractionTest) returnAfterInPlaceChange(cores int, initial, migrated string) {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-core-fraction-return", cores, initial)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for VM agent to be ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)
	skipIfDisksAreNotLiveMigratable(ctx, t.Framework, t.VD)

	By(fmt.Sprintf("Applying %s by live migration", migrated))
	disableInPlaceResizeOnKVVMI(ctx, t.Framework, t.VM)
	t.patchCoreFraction(ctx, migrated)
	t.untilHotplugMigrationsCompleted(ctx, 1, framework.MaxTimeout)
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, migrated), framework.MiddleTimeout)

	By(fmt.Sprintf("Applying %s in place in the same launcher pod", initial))
	enableInPlaceResizeOnKVVMI(ctx, t.Framework, t.VM)
	t.patchCoreFraction(ctx, initial)
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, initial), framework.MiddleTimeout)
	// The pod request changes before the runtime finishes the resize; disabling the in-place
	// resize in the middle of it leaves the change pending for good.
	t.untilCPUChangeApplied(ctx, framework.MiddleTimeout)

	By(fmt.Sprintf("Returning to %s on a node that cannot apply it in place", migrated))
	// The launcher pod was produced by the migration for this very value: that migration must not
	// be taken for one that did not apply it.
	disableInPlaceResizeOnKVVMI(ctx, t.Framework, t.VM)
	t.patchCoreFraction(ctx, migrated)
	t.untilHotplugMigrationsCompleted(ctx, 2, framework.MaxTimeout)
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, migrated), framework.MiddleTimeout)

	Expect(t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)).To(Succeed())
	Expect(t.VM.Status.RestartAwaitingChanges).To(BeEmpty(), "the change applied by live migration should not ask for a restart")
}

// enableInPlaceResizeOnKVVMI drops the annotation disableInPlaceResizeOnKVVMI sets.
func enableInPlaceResizeOnKVVMI(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) {
	GinkgoHelper()

	kvvmi := &unstructured.Unstructured{}
	kvvmi.SetGroupVersionKind(kvvmiGVK())
	kvvmi.SetNamespace(vm.Namespace)
	kvvmi.SetName(vm.Name)

	patch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{disableInPlaceResizeAnn: nil},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(f.ControllerSAClient().Patch(ctx, kvvmi, crclient.RawPatch(types.MergePatchType, patch))).To(Succeed(),
		"in-place resize should be enabled on the internal VirtualMachineInstance %s/%s", vm.Namespace, vm.Name)
}

func (t *coreFractionTest) patchCoreFraction(ctx context.Context, coreFraction string) {
	GinkgoHelper()

	patch, err := json.Marshal([]map[string]interface{}{{
		"op":    "replace",
		"path":  "/spec/cpu/coreFraction",
		"value": coreFraction,
	}})
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))).To(Succeed())
}

// untilHotplugMigrationsCreated waits until n live migrations to apply CPU and memory changes of
// the VM exist, whatever their phase.
func (t *coreFractionTest) untilHotplugMigrationsCreated(ctx context.Context, n int, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vmops := &v1alpha2.VirtualMachineOperationList{}
		g.Expect(t.Framework.GenericClient().List(ctx, vmops, crclient.InNamespace(t.VM.Namespace))).To(Succeed())
		created := 0
		for _, vmop := range vmops.Items {
			if vmop.Spec.VirtualMachine == t.VM.Name && strings.HasPrefix(vmop.Name, commonvmop.HotplugResourcesPrefix) {
				created++
			}
		}
		g.Expect(created).To(BeNumerically(">=", n))
	}).WithTimeout(timeout).WithPolling(2*time.Second).Should(Succeed(),
		"%d live migrations should be created for the CPU changes of VM %s/%s", n, t.VM.Namespace, t.VM.Name)
}

// untilCPUChangeApplied waits until the runtime has applied the CPU change to the running instance.
// KubeVirt resizes the launcher pod only while it reports the change pending, so waiting for the
// pod request first and for the condition to clear next does not race.
func (t *coreFractionTest) untilCPUChangeApplied(ctx context.Context, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		kvvmi := &unstructured.Unstructured{}
		kvvmi.SetGroupVersionKind(kvvmiGVK())
		g.Expect(t.Framework.ControllerSAClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), kvvmi)).To(Succeed())
		conds, _, err := unstructured.NestedSlice(kvvmi.Object, "status", "conditions")
		g.Expect(err).NotTo(HaveOccurred())
		for _, c := range conds {
			cond, _ := c.(map[string]any)
			g.Expect(cond["type"] == string(virtv1.VirtualMachineInstanceVCPUChange) && cond["status"] == string(corev1.ConditionTrue)).
				To(BeFalse(), "the CPU change should not be pending")
		}
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed(),
		"the CPU change of VM %s/%s should be applied by the runtime", t.VM.Namespace, t.VM.Name)
}

// beNonMigratable reports the VM status says it cannot be migrated.
func beNonMigratable() vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond, ok := conditions.GetCondition(vmcondition.TypeMigratable, vm.Status.Conditions)
		return ok && cond.Status == metav1.ConditionFalse, nil
	}
}

// untilHotplugMigrationsCompleted waits until n live migrations created to apply CPU and memory
// changes of the VM have completed.
func (t *coreFractionTest) untilHotplugMigrationsCompleted(ctx context.Context, n int, timeout time.Duration) {
	GinkgoHelper()

	deadline := time.Now().Add(timeout)
	for {
		vmops := &v1alpha2.VirtualMachineOperationList{}
		Expect(t.Framework.GenericClient().List(ctx, vmops, crclient.InNamespace(t.VM.Namespace))).To(Succeed())
		completed := 0
		for _, vmop := range vmops.Items {
			if vmop.Spec.VirtualMachine != t.VM.Name || !strings.HasPrefix(vmop.Name, commonvmop.HotplugResourcesPrefix) {
				continue
			}
			switch vmop.Status.Phase {
			case v1alpha2.VMOPPhaseFailed:
				util.SkipIfKnownMigrationFailureWithContext(ctx, t.VM)
				Fail(fmt.Sprintf("the live migration %s/%s failed", vmop.Namespace, vmop.Name))
			case v1alpha2.VMOPPhaseCompleted:
				completed++
			}
		}
		if completed >= n {
			Expect(completed).To(Equal(n), "no extra live migration should be created for VM %s/%s", t.VM.Namespace, t.VM.Name)
			return
		}
		if time.Now().After(deadline) {
			Fail(fmt.Sprintf("%d live migrations should apply the CPU changes of VM %s/%s, %d completed", n, t.VM.Namespace, t.VM.Name, completed))
		}
		time.Sleep(5 * time.Second)
	}
}

// haveRecommendedAbove reports the autoscaler recommends a coreFraction above previous and
// stores it into recommended.
func haveRecommendedAbove(previous string, recommended *string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		current := sizingpolicy.RecommendedCoreFraction(vm)
		if current == "" {
			return false, nil
		}
		currentPct, err := strconv.Atoi(strings.TrimSuffix(current, "%"))
		if err != nil {
			return false, fmt.Errorf("malformed recommended coreFraction %q: %w", current, err)
		}
		previousPct, err := strconv.Atoi(strings.TrimSuffix(previous, "%"))
		if err != nil {
			return false, fmt.Errorf("malformed coreFraction %q: %w", previous, err)
		}
		if currentPct <= previousPct {
			return false, nil
		}
		*recommended = current
		return true, nil
	}
}

func (t *coreFractionTest) generateResources(vmName string, cores int, coreFraction string) {
	t.VD = object.NewVDFromCVI(fmt.Sprintf("vd-%s", vmName), t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS)
	t.VM = object.NewMinimalVM("", t.Framework.Namespace().Name,
		vmbuilder.WithName(vmName),
		vmbuilder.WithCPU(cores, ptr.To(coreFraction)),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: t.VD.Name,
		}),
	)
}

func vpaGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "autoscaling.k8s.io", Version: "v1", Resource: "verticalpodautoscalers"}
}

func kvvmGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "internal.virtualization.deckhouse.io",
		Version: "v1",
		Kind:    "InternalVirtualizationVirtualMachine",
	}
}

// untilVPAExists waits until the autoscaler has created the VM's VPA: same namespace as
// the VM, name derived from its UID. The VPA type is not in the e2e scheme, so it is
// observed as unstructured through a dynamic watch.
func (t *coreFractionTest) untilVPAExists(ctx context.Context, timeout time.Duration) {
	GinkgoHelper()

	vpaName := commonvm.VerticalPodAutoscalerName(t.VM)
	_, err := observer.WaitForFirst(ctx,
		observer.DynamicWatcher(framework.GetClients().DynamicClient(), vpaGVR(), t.VM.Namespace),
		timeout,
		func(vpa *unstructured.Unstructured) bool {
			return vpa.GetName() == vpaName
		})
	Expect(err).NotTo(HaveOccurred(), "the autoscaler should create VPA %s/%s", t.VM.Namespace, vpaName)
}

// recommendationOverrideCPU builds the JSON RecommendedPodResources the controller reads
// from the override annotation: a single compute-container CPU recommendation. The
// container name must end with "compute" to match the controller's compute-container check.
func recommendationOverrideCPU(target, lower string) string {
	return fmt.Sprintf(
		`{"containerRecommendations":[{"containerName":"d8v-compute","target":{"cpu":%q},"lowerBound":{"cpu":%q},"upperBound":{"cpu":"40000m"}}]}`,
		target, lower,
	)
}

// patchKVVMRecommendationOverride sets the override annotation on the VM's internal
// VirtualMachine with a JSON merge patch, so the controller acts on the pinned
// recommendation. The internal API group only admits the module's own ServiceAccounts,
// so the patch goes through the impersonating client.
func (t *coreFractionTest) patchKVVMRecommendationOverride(ctx context.Context, override string) {
	GinkgoHelper()

	kvvm := &unstructured.Unstructured{}
	kvvm.SetGroupVersionKind(kvvmGVK())
	kvvm.SetNamespace(t.VM.Namespace)
	kvvm.SetName(t.VM.Name)

	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				annotations.AnnRecommendationOverride: override,
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Framework.ControllerSAClient().Patch(ctx, kvvm, crclient.RawPatch(types.MergePatchType, patch))).To(Succeed())
}

// untilPodCPURequest waits until the VM's running virt-launcher pod requests the
// expected CPU (in millicores) on its compute container. The in-place resize
// updates the pod object, so the change arrives as a watch event on the same pod.
func (t *coreFractionTest) untilPodCPURequest(ctx context.Context, expectedMilli int64, timeout time.Duration) {
	GinkgoHelper()

	// Only the active pod counts: the source pod of a completed migration keeps running for a while
	// with the request the VM had before.
	Eventually(func(g Gomega) {
		_, pod, err := util.GetVirtualMachineAndActivePod(ctx, t.Framework, t.VM)
		g.Expect(err).NotTo(HaveOccurred())
		req, ok := computeContainerCPURequest(pod)
		g.Expect(ok).To(BeTrue())
		g.Expect(req.MilliValue()).To(Equal(expectedMilli))
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed(),
		"the compute container of the active virt-launcher pod of VM %s/%s should request %dm CPU",
		t.VM.Namespace, t.VM.Name, expectedMilli)
}

func computeContainerCPURequest(pod *corev1.Pod) (resource.Quantity, bool) {
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		if !strings.HasSuffix(c.Name, "compute") {
			continue
		}
		req, ok := c.Resources.Requests[corev1.ResourceCPU]
		return req, ok
	}
	return resource.Quantity{}, false
}

// expectedCPURequestMilli returns cores*coreFraction in millicores: cores*1000m is
// 100%, so a percent point is cores*10m.
func expectedCPURequestMilli(cores int, coreFraction string) int64 {
	return int64(cores) * 10 * int64(percent(coreFraction))
}

func percent(coreFraction string) int {
	GinkgoHelper()
	v, err := strconv.Atoi(strings.TrimSuffix(coreFraction, "%"))
	Expect(err).NotTo(HaveOccurred())
	return v
}

// haveAppliedCoreFraction reports the VM status carries exactly the given
// coreFraction.
func haveAppliedCoreFraction(expected string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		return vm.Status.Resources.CPU.CoreFraction == expected, nil
	}
}

// haveRaisedAndAppliedCoreFraction reports the autoscaler recommended a
// coreFraction above initial and the VM status applied exactly that value.
// The applied value of the satisfying observation is stored into applied.
func haveRaisedAndAppliedCoreFraction(initial string, applied *string) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		recommended := sizingpolicy.RecommendedCoreFraction(vm)
		if recommended == "" || vm.Status.Resources.CPU.CoreFraction != recommended {
			return false, nil
		}
		recommendedPct, err := strconv.Atoi(strings.TrimSuffix(recommended, "%"))
		if err != nil {
			return false, fmt.Errorf("malformed recommended coreFraction %q: %w", recommended, err)
		}
		initialPct, err := strconv.Atoi(strings.TrimSuffix(initial, "%"))
		if err != nil {
			return false, fmt.Errorf("malformed initial coreFraction %q: %w", initial, err)
		}
		if recommendedPct <= initialPct {
			return false, nil
		}
		*applied = vm.Status.Resources.CPU.CoreFraction
		return true, nil
	}
}
