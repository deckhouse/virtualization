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
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
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
			t.applyExplicitCoreFractionChange(1, "10%", "50%")
		})
	})

	Context("AutoCoreFraction", Label(precheck.HotplugInPlaceResizePrecheck), Label(precheck.PrecheckVerticalPodAutoscaler), func() {
		It("should autoscale coreFraction from a pinned VPA recommendation", func() {
			t.autoscaleCoreFractionViaRecommendation(2)
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
	t.generateAlpineResources("vm-core-fraction", cores, initial)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())

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
	untilVMCoreFractionApplied(crclient.ObjectKeyFromObject(t.VM), changed, framework.MiddleTimeout)
	util.ExpectNoVMOperationsForVirtualMachine(ctx, t.Framework, t.VM)
	util.ExpectVMOnNode(ctx, t.Framework, t.VM, initialNode)

	By("Checking the pod CPU request follows the new coreFraction")
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, changed), framework.MiddleTimeout)
}

func (t *coreFractionTest) autoscaleCoreFractionViaRecommendation(cores int) {
	ctx := context.Background()

	By("Environment preparation")
	t.generateAlpineResources("vm-core-fraction-auto", cores, v1alpha2.CoreFractionAuto)
	err := t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for VM agent to be ready")
	util.UntilVMAgentReady(ctx, crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

	By("Reading the seeded coreFraction")
	err = t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
	Expect(err).NotTo(HaveOccurred())
	initialFraction := t.VM.Status.AutoCoreFraction
	Expect(initialFraction).NotTo(BeEmpty(), "autoscaler should seed status.autoCoreFraction")

	By("Waiting until the autoscaler creates the VPA")
	t.untilVPAExists(ctx, framework.MiddleTimeout)

	By("Pinning a high CPU recommendation via the override annotation")
	// Bypass the vpa-recommender's slow, decaying CPU histogram: with a lowerBound above
	// the VM's current CPU request the autoscaler must raise coreFraction, and with a
	// target far beyond capacity the desired value snaps to the largest Burstable step
	// the sizing policy allows. This exercises the recommendation-to-hotplug seam without
	// any in-guest load.
	t.patchVPARecommendationOverride(ctx, recommendationOverrideCPU("20000m", "20000m", "40000m"))

	By("Waiting until the autoscaler raises coreFraction from the recommendation")
	var applied string
	Eventually(func(g Gomega) {
		g.Expect(t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)).To(Succeed())
		desired := t.VM.Status.AutoCoreFraction
		applied = t.VM.Status.Resources.CPU.CoreFraction
		g.Expect(percent(desired)).To(BeNumerically(">", percent(initialFraction)),
			"desired coreFraction should grow from the pinned recommendation")
		g.Expect(applied).To(Equal(desired), "desired coreFraction should be applied")
	}).WithTimeout(framework.MiddleTimeout).WithPolling(time.Second).Should(Succeed())

	By("Checking the pod CPU request follows the applied coreFraction")
	t.untilPodCPURequest(ctx, expectedCPURequestMilli(cores, applied), framework.MiddleTimeout)
}

func (t *coreFractionTest) generateAlpineResources(vmName string, cores int, coreFraction string) {
	t.VD = object.NewVDFromCVI(fmt.Sprintf("vd-%s", vmName), t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS)
	t.VM = object.NewMinimalVM("", t.Framework.Namespace().Name,
		vmbuilder.WithName(vmName),
		vmbuilder.WithCPU(cores, ptr.To(coreFraction)),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: t.VD.Name,
		}),
	)
}

func vpaGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "autoscaling.k8s.io", Version: "v1", Kind: "VerticalPodAutoscaler"}
}

// untilVPAExists waits until the autoscaler has created the VM's VPA (same name and
// namespace as the VM). The VPA type is not in the e2e scheme, so it is read as
// unstructured via the dynamic REST mapper.
func (t *coreFractionTest) untilVPAExists(ctx context.Context, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vpa := &unstructured.Unstructured{}
		vpa.SetGroupVersionKind(vpaGVK())
		g.Expect(t.Framework.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), vpa)).To(Succeed())
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
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

// patchVPARecommendationOverride sets the override annotation on the VM's VPA with a
// JSON merge patch, so the controller acts on the pinned recommendation.
func (t *coreFractionTest) patchVPARecommendationOverride(ctx context.Context, override string) {
	GinkgoHelper()

	vpa := &unstructured.Unstructured{}
	vpa.SetGroupVersionKind(vpaGVK())
	vpa.SetNamespace(t.VM.Namespace)
	vpa.SetName(t.VM.Name)

	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				annotations.AnnRecommendationOverride: override,
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Framework.GenericClient().Patch(ctx, vpa, crclient.RawPatch(types.MergePatchType, patch))).To(Succeed())
}

// untilPodCPURequest waits until the active pod's compute container requests the
// expected CPU (in millicores).
func (t *coreFractionTest) untilPodCPURequest(ctx context.Context, expectedMilli int64, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		_, pod, err := util.GetVirtualMachineAndActivePod(ctx, t.Framework, t.VM)
		g.Expect(err).NotTo(HaveOccurred())

		req, ok := computeContainerCPURequest(pod)
		g.Expect(ok).To(BeTrue(), "compute container should request CPU")
		g.Expect(req.MilliValue()).To(Equal(expectedMilli))
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
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

func untilVMCoreFractionApplied(key crclient.ObjectKey, expected string, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm, err := framework.GetClients().VirtClient().VirtualMachines(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vm.Status.Resources.CPU.CoreFraction).To(Equal(expected))
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}
