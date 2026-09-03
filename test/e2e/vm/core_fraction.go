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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
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
