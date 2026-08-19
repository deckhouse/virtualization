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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	t.patchKVVMRecommendationOverride(ctx, recommendationOverrideCPU("20000m", "20000m", "40000m"))

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
func recommendationOverrideCPU(target, lower, upper string) string {
	return fmt.Sprintf(
		`{"containerRecommendations":[{"containerName":"d8v-compute","target":{"cpu":%q},"lowerBound":{"cpu":%q},"upperBound":{"cpu":%q}}]}`,
		target, lower, upper,
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

	_, err := observer.WaitForFirst(ctx,
		t.Framework.KubeClient().CoreV1().Pods(t.VM.Namespace),
		timeout,
		func(pod *corev1.Pod) bool {
			if pod.Labels["kubevirt.internal.virtualization.deckhouse.io"] != "virt-launcher" ||
				pod.Labels["vm.kubevirt.internal.virtualization.deckhouse.io/name"] != t.VM.Name {
				return false
			}
			if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
				return false
			}
			req, ok := computeContainerCPURequest(pod)
			return ok && req.MilliValue() == expectedMilli
		})
	Expect(err).NotTo(HaveOccurred(),
		"the compute container of the virt-launcher pod of VM %s/%s should request %dm CPU",
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
