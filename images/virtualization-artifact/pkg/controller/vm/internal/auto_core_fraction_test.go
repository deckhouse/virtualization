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

package internal

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("AutoCoreFractionHandler", func() {
	ctx := testutil.ContextBackgroundWithNoOpLogger()

	const (
		vmName    = "vm"
		vmUID     = "b0a5f3f4-4a2c-4c1a-9a3b-6f2f0f9a7c11"
		namespace = "default"
		className = "class"
		// The VPA the handler owns is named after the VM UID, not the VM name.
		vpaName = "vm-" + vmUID
	)

	gates := func(autoscaler, inPlaceResize bool) featuregate.FeatureGate {
		g, setFromMap, err := featuregates.NewUnlocked()
		Expect(err).NotTo(HaveOccurred())
		Expect(setFromMap(map[string]bool{
			string(featuregates.VerticalVirtualMachineAutoscaler):     autoscaler,
			string(featuregates.HotplugCPUAndMemoryWithInPlaceResize): inPlaceResize,
		})).To(Succeed())
		return g
	}

	gate := func(enabled bool) featuregate.FeatureGate { return gates(enabled, enabled) }

	// classWith builds a class whose sizing policy for 1..8 cores allows the given fractions.
	classWith := func(fractions ...v1alpha2.CoreFractionValue) *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Name: className},
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{{
					Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 8},
					CoreFractions: fractions,
				}},
			},
		}
	}

	expectCondition := func(vm *v1alpha2.VirtualMachine, status metav1.ConditionStatus, reason vmcondition.CoreFractionAutoscalingReason) metav1.Condition {
		GinkgoHelper()
		cond, found := conditions.GetCondition(vmcondition.TypeCoreFractionAutoscaling, vm.Status.Conditions)
		Expect(found).To(BeTrue())
		Expect(cond.Status).To(Equal(status))
		Expect(cond.Reason).To(Equal(reason.String()))
		return cond
	}

	newRecorder := func() *eventrecord.EventRecorderLoggerMock {
		var rec *eventrecord.EventRecorderLoggerMock
		rec = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...interface{}) {},
			WithLoggingFunc: func(_ eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return rec },
		}
		return rec
	}

	newVM := func(coreFraction, autoCoreFraction string) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace, UID: vmUID},
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: className,
				CPU:                     v1alpha2.CPUSpec{Cores: 4, CoreFraction: coreFraction},
			},
		}
		sizingpolicy.SetRecommendedCoreFraction(vm, autoCoreFraction)
		return vm
	}

	newClass := func() *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Name: className},
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{{
					Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 8},
					CoreFractions: []v1alpha2.CoreFractionValue{25, 50, 75, 100},
				}},
			},
		}
	}

	milli := func(m int64) resource.Quantity { return *resource.NewMilliQuantity(m, resource.DecimalSI) }

	// newVPA builds a VPA carrying a CPU recommendation for the compute container.
	newVPA := func(target, lower, upper int64) *vpav1.VerticalPodAutoscaler {
		return &vpav1.VerticalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: vpaName, Namespace: namespace},
			Status: vpav1.VerticalPodAutoscalerStatus{
				Recommendation: &vpav1.RecommendedPodResources{
					ContainerRecommendations: []vpav1.RecommendedContainerResources{{
						ContainerName: "compute",
						Target:        corev1.ResourceList{corev1.ResourceCPU: milli(target)},
						LowerBound:    corev1.ResourceList{corev1.ResourceCPU: milli(lower)},
						UpperBound:    corev1.ResourceList{corev1.ResourceCPU: milli(upper)},
					}},
				},
			},
		}
	}

	handle := func(g featuregate.FeatureGate, rec eventrecord.EventRecorderLogger, vm *v1alpha2.VirtualMachine, objs ...client.Object) (client.WithWatch, *v1alpha2.VirtualMachine) {
		fakeClient, _, vmState := setupEnvironment(vm, objs...)
		h := NewAutoCoreFractionHandler(fakeClient, rec, fakeClient.Scheme(), service.NewCoreFractionService(), g)
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		return fakeClient, vmState.VirtualMachine().Changed()
	}

	getVPA := func(c client.Client) (*vpav1.VerticalPodAutoscaler, bool) {
		obj := &vpav1.VerticalPodAutoscaler{}
		err := c.Get(ctx, types.NamespacedName{Name: vpaName, Namespace: namespace}, obj)
		if apierrors.IsNotFound(err) {
			return nil, false
		}
		Expect(err).NotTo(HaveOccurred())
		return obj, true
	}

	It("seeds the step closest to 10% and creates the VPA on first sight", func() {
		// Policy steps [25,50,75,99]; 25 is the closest to the 10% seed target.
		fakeClient, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, ""), newClass())
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("25%"))
		_, ok := getVPA(fakeClient)
		Expect(ok).To(BeTrue())
	})

	It("scales up when the current request is below the lower bound", func() {
		rec := newRecorder()
		// current 10% -> 400m < 1000 lower; target 1400m -> raw 35% -> policy 50%.
		_, changed := handle(gate(true), rec, newVM(v1alpha2.CoreFractionAuto, "10%"), newClass(), newVPA(1400, 1000, 2000))
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("50%"))
		Expect(rec.EventfCalls()).To(HaveLen(1))
	})

	It("acts on the recommendation pinned in the override annotation", func() {
		rec := newRecorder()
		override, err := json.Marshal(&vpav1.RecommendedPodResources{
			ContainerRecommendations: []vpav1.RecommendedContainerResources{{
				ContainerName: "compute",
				Target:        corev1.ResourceList{corev1.ResourceCPU: milli(1400)},
				LowerBound:    corev1.ResourceList{corev1.ResourceCPU: milli(1000)},
				UpperBound:    corev1.ResourceList{corev1.ResourceCPU: milli(2000)},
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		// VPA carries no recommender status; the override annotation on the internal VM
		// alone drives it.
		// current 10% -> 400m < 1000 lower; target 1400m -> raw 35% -> policy 50%.
		vpa := &vpav1.VerticalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: vpaName, Namespace: namespace},
		}
		kvvm := newEmptyKVVM(vmName, namespace)
		kvvm.Annotations = map[string]string{annotations.AnnRecommendationOverride: string(override)}
		_, changed := handle(gate(true), rec, newVM(v1alpha2.CoreFractionAuto, "10%"), newClass(), vpa, kvvm)
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("50%"))
		Expect(rec.EventfCalls()).To(HaveLen(1))
	})

	It("ignores a malformed override annotation and falls back to the recommender status", func() {
		rec := newRecorder()
		// status recommends holding (current 50% -> 2000m inside [1600, 2400]).
		vpa := newVPA(2000, 1600, 2400)
		kvvm := newEmptyKVVM(vmName, namespace)
		kvvm.Annotations = map[string]string{annotations.AnnRecommendationOverride: "not-json"}
		_, changed := handle(gate(true), rec, newVM(v1alpha2.CoreFractionAuto, "50%"), newClass(), vpa, kvvm)
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("50%"))
		Expect(rec.EventfCalls()).To(BeEmpty())
	})

	It("holds still while the current request is within the recommended range", func() {
		rec := newRecorder()
		// current 50% -> 2000m, inside [1600, 2400].
		_, changed := handle(gate(true), rec, newVM(v1alpha2.CoreFractionAuto, "50%"), newClass(), newVPA(2000, 1600, 2400))
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("50%"))
		Expect(rec.EventfCalls()).To(BeEmpty())
	})

	It("retracts the driven value and deletes the VPA when autoscaling is off", func() {
		fakeClient, changed := handle(gate(true), newRecorder(), newVM("50%", "75%"), newClass(), newVPA(2000, 1600, 2400))
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(BeEmpty())
		_, ok := getVPA(fakeClient)
		Expect(ok).To(BeFalse())
	})

	It("ignores a same-named VPA the user owns", func() {
		// A VPA named after the VM belongs to whoever created it: the handler must neither
		// read its recommendation nor delete it when autoscaling is switched off.
		foreign := newVPA(1400, 1000, 2000)
		foreign.Name = vmName

		_, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, "10%"), newClass(), foreign)
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("10%"))

		fakeClient, changed := handle(gate(true), newRecorder(), newVM("50%", "75%"), newClass(), foreign)
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(BeEmpty())
		obj := &vpav1.VerticalPodAutoscaler{}
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, obj)).To(Succeed())
	})

	It("neither seeds nor creates the VPA when the autoscaler feature is disabled", func() {
		fakeClient, changed := handle(gate(false), newRecorder(), newVM(v1alpha2.CoreFractionAuto, ""), newClass())
		Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(BeEmpty())
		_, ok := getVPA(fakeClient)
		Expect(ok).To(BeFalse())
	})

	It("reports the enabled condition once a recommendation is there", func() {
		_, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, "50%"), newClass(), newVPA(2000, 1600, 2400))

		cond := expectCondition(changed, metav1.ConditionTrue, vmcondition.ReasonCoreFractionAutoscalingEnabled)
		Expect(cond.Message).To(Equal("The CPU core fraction is selected automatically."))
	})

	Context("waiting for a recommendation", func() {
		expectWaiting := func(changed *v1alpha2.VirtualMachine) {
			GinkgoHelper()
			cond := expectCondition(changed, metav1.ConditionTrue, vmcondition.ReasonWaitingForRecommendation)
			Expect(cond.Message).To(Equal("The CPU core fraction is selected automatically. " +
				"The initial value is used until enough CPU usage data is collected."))
		}

		It("reports it while seeding the initial value", func() {
			_, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, ""), newClass())
			Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("25%"))
			expectWaiting(changed)
		})

		It("reports it while the VPA carries no recommendation", func() {
			vpa := &vpav1.VerticalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: vpaName, Namespace: namespace}}
			_, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, "25%"), newClass(), vpa)
			expectWaiting(changed)
		})

		It("reports it while the VirtualMachineClass does not exist", func() {
			_, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, "25%"))
			expectWaiting(changed)
		})
	})

	Context("autoscaling is unavailable", func() {
		It("keeps the core fraction and reports the disabled autoscaler", func() {
			// The feature went off under a running Auto VM: nothing drives its core fraction
			// any more, but the VM keeps the value it has.
			fakeClient, changed := handle(gates(false, true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, "50%"), newClass(), newVPA(2000, 1600, 2400))

			Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("50%"))
			_, ok := getVPA(fakeClient)
			Expect(ok).To(BeFalse())

			cond := expectCondition(changed, metav1.ConditionFalse, vmcondition.ReasonCoreFractionAutoscalingDisabled)
			Expect(cond.Message).To(Equal("The CPU core fraction cannot be selected automatically: " +
				"vertical autoscaling of virtual machines is disabled. The virtual machine keeps its current core fraction. " +
				"To manage it yourself, set an explicit value in the specification."))
		})

		It("keeps the core fraction and reports the disabled in-place resize", func() {
			fakeClient, changed := handle(gates(true, false), newRecorder(), newVM(v1alpha2.CoreFractionAuto, "50%"), newClass(), newVPA(2000, 1600, 2400))

			Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("50%"))
			_, ok := getVPA(fakeClient)
			Expect(ok).To(BeFalse())

			cond := expectCondition(changed, metav1.ConditionFalse, vmcondition.ReasonInPlaceResizeDisabled)
			Expect(cond.Message).To(Equal("The CPU core fraction cannot be selected automatically: " +
				"in-place resizing of virtual machine resources is disabled. The virtual machine keeps its current core fraction. " +
				"To manage it yourself, set an explicit value in the specification."))
		})

		It("keeps the core fraction when the sizing policy was narrowed under the VM", func() {
			// The webhook rejects "Auto" against such a policy, but the class can be narrowed
			// afterwards — the VM sorts it out for itself instead of blocking the class.
			fakeClient, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, "50%"), classWith(50, 100), newVPA(2000, 1600, 2400))

			Expect(sizingpolicy.RecommendedCoreFraction(changed)).To(Equal("50%"))
			_, ok := getVPA(fakeClient)
			Expect(ok).To(BeFalse())

			cond := expectCondition(changed, metav1.ConditionFalse, vmcondition.ReasonSizingPolicyHasNoSteps)
			Expect(cond.Message).To(Equal(`The CPU core fraction cannot be selected automatically: ` +
				`the sizing policy of the VirtualMachineClass "class" allows a single core fraction below 100%. ` +
				`The virtual machine keeps its current core fraction. ` +
				`Ask the administrator to allow more values, or set an explicit one in the specification.`))
		})

		It("says the policy allows nothing when it only allows 100%", func() {
			_, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, "50%"), classWith(100), newVPA(2000, 1600, 2400))

			cond := expectCondition(changed, metav1.ConditionFalse, vmcondition.ReasonSizingPolicyHasNoSteps)
			Expect(cond.Message).To(ContainSubstring("allows no core fraction below 100%"))
		})
	})

	It("carries no condition for a VM with an explicit core fraction", func() {
		for _, g := range []featuregate.FeatureGate{gate(true), gate(false)} {
			_, changed := handle(g, newRecorder(), newVM("50%", ""), newClass())
			_, found := conditions.GetCondition(vmcondition.TypeCoreFractionAutoscaling, changed.Status.Conditions)
			Expect(found).To(BeFalse())
		}
	})
})
