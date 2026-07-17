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

package validators_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/validators"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Default core fraction validator", func() {
	ctx := testutil.ContextBackgroundWithNoOpLogger()

	gate := func(enabled bool) featuregate.FeatureGate {
		g, setFromMap, err := featuregates.NewUnlocked()
		Expect(err).NotTo(HaveOccurred())
		Expect(setFromMap(map[string]bool{
			string(featuregates.VerticalVirtualMachineAutoscaler):     enabled,
			string(featuregates.HotplugCPUAndMemoryWithInPlaceResize): enabled,
		})).To(Succeed())
		return g
	}

	newVMClass := func(defaultCoreFraction *intstr.IntOrString, coreFractions ...int) *v1alpha2.VirtualMachineClass {
		cfs := make([]v1alpha2.CoreFractionValue, len(coreFractions))
		for i, cf := range coreFractions {
			cfs[i] = v1alpha2.CoreFractionValue(cf)
		}
		return &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Name: "test-class"},
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{{
					Cores:               &v1alpha2.SizingPolicyCores{Min: 1, Max: 8},
					CoreFractions:       cfs,
					DefaultCoreFraction: defaultCoreFraction,
				}},
			},
		}
	}

	auto := func() *intstr.IntOrString { return ptr.To(intstr.FromString(string(v1alpha2.CoreFractionAuto))) }

	Context("a numeric default", func() {
		It("accepts a value listed in coreFractions", func() {
			_, err := validators.NewDefaultCoreFractionValidator(gate(true)).
				ValidateCreate(ctx, newVMClass(ptr.To(intstr.FromInt32(50)), 25, 50, 100))
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects a value missing from coreFractions", func() {
			_, err := validators.NewDefaultCoreFractionValidator(gate(true)).
				ValidateCreate(ctx, newVMClass(ptr.To(intstr.FromInt32(30)), 25, 50, 100))
			Expect(err).To(MatchError(ContainSubstring("is not among the allowed core fractions")))
		})

		It("accepts any value when the policy lists no coreFractions", func() {
			_, err := validators.NewDefaultCoreFractionValidator(gate(true)).
				ValidateCreate(ctx, newVMClass(ptr.To(intstr.FromInt32(30))))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("the Auto default", func() {
		It("is accepted without being listed in coreFractions", func() {
			_, err := validators.NewDefaultCoreFractionValidator(gate(true)).
				ValidateCreate(ctx, newVMClass(auto(), 25, 50, 100))
			Expect(err).NotTo(HaveOccurred())
		})

		It("is rejected when the autoscaling features are disabled", func() {
			_, err := validators.NewDefaultCoreFractionValidator(gate(false)).
				ValidateCreate(ctx, newVMClass(auto(), 25, 50, 100))
			Expect(err).To(MatchError(ContainSubstring("vertical VirtualMachine autoscaling is disabled")))
		})

		It("stays editable once set, even with the features disabled", func() {
			old := newVMClass(auto(), 25, 50, 100)
			updated := newVMClass(auto(), 25, 50, 75, 100)
			_, err := validators.NewDefaultCoreFractionValidator(gate(false)).ValidateUpdate(ctx, old, updated)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects any other string value", func() {
			_, err := validators.NewDefaultCoreFractionValidator(gate(true)).
				ValidateCreate(ctx, newVMClass(ptr.To(intstr.FromString("50%"))))
			Expect(err).To(MatchError(ContainSubstring("is invalid")))
		})

		It("is accepted when the policy constrains no core fraction", func() {
			_, err := validators.NewDefaultCoreFractionValidator(gate(true)).ValidateCreate(ctx, newVMClass(auto()))
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("is rejected when the policy leaves nothing to scale between",
			func(coreFractions ...int) {
				_, err := validators.NewDefaultCoreFractionValidator(gate(true)).
					ValidateCreate(ctx, newVMClass(auto(), coreFractions...))
				Expect(err).To(MatchError(ContainSubstring("core fractions to choose from")))
			},
			// 100% is never used automatically, so it does not count as a step.
			Entry("a single core fraction", 50),
			Entry("a single core fraction plus 100%", 50, 100),
			Entry("only 100%", 100),
		)
	})
})
