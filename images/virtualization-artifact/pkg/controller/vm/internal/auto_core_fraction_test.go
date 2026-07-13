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
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("AutoCoreFractionHandler", func() {
	ctx := testutil.ContextBackgroundWithNoOpLogger()

	const (
		vmName    = "vm"
		namespace = "default"
		className = "class"
	)

	gate := func(enabled bool) featuregate.FeatureGate {
		g, setFromMap, err := featuregates.NewUnlocked()
		Expect(err).NotTo(HaveOccurred())
		Expect(setFromMap(map[string]bool{
			string(featuregates.VerticalVirtualMachineAutoscaler):     enabled,
			string(featuregates.HotplugCPUAndMemoryWithInPlaceResize): enabled,
		})).To(Succeed())
		return g
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
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: className,
				CPU:                     v1alpha2.CPUSpec{Cores: 4, CoreFraction: coreFraction},
			},
			Status: v1alpha2.VirtualMachineStatus{AutoCoreFraction: autoCoreFraction},
		}
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
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
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
		err := c.Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, obj)
		if apierrors.IsNotFound(err) {
			return nil, false
		}
		Expect(err).NotTo(HaveOccurred())
		return obj, true
	}

	It("seeds a middle Burstable coreFraction and creates the VPA on first sight", func() {
		fakeClient, changed := handle(gate(true), newRecorder(), newVM(v1alpha2.CoreFractionAuto, ""), newClass())
		Expect(changed.Status.AutoCoreFraction).To(Equal("50%"))
		_, ok := getVPA(fakeClient)
		Expect(ok).To(BeTrue())
	})

	It("scales up when the current request is below the lower bound", func() {
		rec := newRecorder()
		// current 10% -> 400m < 1000 lower; target 1400m -> raw 35% -> policy 50%.
		_, changed := handle(gate(true), rec, newVM(v1alpha2.CoreFractionAuto, "10%"), newClass(), newVPA(1400, 1000, 2000))
		Expect(changed.Status.AutoCoreFraction).To(Equal("50%"))
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

		// VPA carries no recommender status; the override annotation alone drives it.
		// current 10% -> 400m < 1000 lower; target 1400m -> raw 35% -> policy 50%.
		vpa := &vpav1.VerticalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmName,
				Namespace:   namespace,
				Annotations: map[string]string{annotations.AnnRecommendationOverride: string(override)},
			},
		}
		_, changed := handle(gate(true), rec, newVM(v1alpha2.CoreFractionAuto, "10%"), newClass(), vpa)
		Expect(changed.Status.AutoCoreFraction).To(Equal("50%"))
		Expect(rec.EventfCalls()).To(HaveLen(1))
	})

	It("ignores a malformed override annotation and falls back to the recommender status", func() {
		rec := newRecorder()
		// status recommends holding (current 50% -> 2000m inside [1600, 2400]).
		vpa := newVPA(2000, 1600, 2400)
		vpa.Annotations = map[string]string{annotations.AnnRecommendationOverride: "not-json"}
		_, changed := handle(gate(true), rec, newVM(v1alpha2.CoreFractionAuto, "50%"), newClass(), vpa)
		Expect(changed.Status.AutoCoreFraction).To(Equal("50%"))
		Expect(rec.EventfCalls()).To(BeEmpty())
	})

	It("holds still while the current request is within the recommended range", func() {
		rec := newRecorder()
		// current 50% -> 2000m, inside [1600, 2400].
		_, changed := handle(gate(true), rec, newVM(v1alpha2.CoreFractionAuto, "50%"), newClass(), newVPA(2000, 1600, 2400))
		Expect(changed.Status.AutoCoreFraction).To(Equal("50%"))
		Expect(rec.EventfCalls()).To(BeEmpty())
	})

	It("retracts the driven value and deletes the VPA when autoscaling is off", func() {
		fakeClient, changed := handle(gate(true), newRecorder(), newVM("50%", "75%"), newClass(), newVPA(2000, 1600, 2400))
		Expect(changed.Status.AutoCoreFraction).To(BeEmpty())
		_, ok := getVPA(fakeClient)
		Expect(ok).To(BeFalse())
	})

	It("is a no-op when the autoscaler feature is disabled", func() {
		fakeClient, changed := handle(gate(false), newRecorder(), newVM(v1alpha2.CoreFractionAuto, ""), newClass())
		Expect(changed.Status.AutoCoreFraction).To(BeEmpty())
		_, ok := getVPA(fakeClient)
		Expect(ok).To(BeFalse())
	})
})
