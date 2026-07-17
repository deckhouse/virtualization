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

package sizingpolicy

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func fractions(vals ...int) []v1alpha2.CoreFractionValue {
	out := make([]v1alpha2.CoreFractionValue, len(vals))
	for i, v := range vals {
		out[i] = v1alpha2.CoreFractionValue(v)
	}
	return out
}

// classWith builds a VMClass with one sizing policy for the [1,max] cores range
// and the given allowed coreFractions.
func classWith(max int, cfs ...int) *v1alpha2.VirtualMachineClass {
	return &v1alpha2.VirtualMachineClass{
		Spec: v1alpha2.VirtualMachineClassSpec{
			SizingPolicies: []v1alpha2.SizingPolicy{
				{
					Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: max},
					CoreFractions: fractions(cfs...),
				},
			},
		},
	}
}

var _ = Describe("FormatCoreFractionValues", func() {
	It("formats values as percentages", func() {
		Expect(FormatCoreFractionValues(fractions(25, 50, 100))).To(Equal([]string{"25%", "50%", "100%"}))
	})

	It("returns an empty slice for no values", func() {
		Expect(FormatCoreFractionValues(nil)).To(BeEmpty())
	})
})

var _ = Describe("MatchSizingPolicy", func() {
	It("returns nil when the class is nil", func() {
		Expect(MatchSizingPolicy(nil, 4)).To(BeNil())
	})

	It("returns the policy whose range contains cores (inclusive bounds)", func() {
		class := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{
					{Cores: &v1alpha2.SizingPolicyCores{Min: 1, Max: 4}, CoreFractions: fractions(25)},
					{Cores: &v1alpha2.SizingPolicyCores{Min: 5, Max: 8}, CoreFractions: fractions(50)},
				},
			},
		}
		Expect(MatchSizingPolicy(class, 1).CoreFractions).To(Equal(fractions(25)))
		Expect(MatchSizingPolicy(class, 4).CoreFractions).To(Equal(fractions(25)))
		Expect(MatchSizingPolicy(class, 5).CoreFractions).To(Equal(fractions(50)))
	})

	It("returns nil when no range matches", func() {
		Expect(MatchSizingPolicy(classWith(2), 4)).To(BeNil())
	})

	It("skips policies without a cores range", func() {
		class := &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{
				SizingPolicies: []v1alpha2.SizingPolicy{{CoreFractions: fractions(10)}},
			},
		}
		Expect(MatchSizingPolicy(class, 4)).To(BeNil())
	})

	It("returns a deep copy that does not alias the class", func() {
		class := classWith(8, 25)
		got := MatchSizingPolicy(class, 4)
		got.CoreFractions[0] = 99
		Expect(class.Spec.SizingPolicies[0].CoreFractions[0]).To(Equal(v1alpha2.CoreFractionValue(25)))
	})
})

var _ = Describe("NeededCoreFraction", func() {
	DescribeTable("inverts a CPU target into the covering fraction",
		func(cores int, targetMilli int64, expected int) {
			Expect(NeededCoreFraction(cores, targetMilli)).To(Equal(expected))
		},
		Entry("exact multiple", 4, int64(2000), 50), // 2000 / (4*10) = 50
		Entry("rounds up", 4, int64(1370), 35),      // ceil(1370/40) = 35
		Entry("rounds up small remainder", 4, int64(2001), 51),
		Entry("clamps below to the minimum", 4, int64(1), 1),
		Entry("clamps above to the autoscaling ceiling", 4, int64(8000), MaxAutoCoreFraction),
		Entry("single core", 1, int64(250), 25),
	)

	It("returns the autoscaling ceiling for a non-positive core count", func() {
		Expect(NeededCoreFraction(0, 1000)).To(Equal(MaxAutoCoreFraction))
		Expect(NeededCoreFraction(-1, 1000)).To(Equal(MaxAutoCoreFraction))
	})
})

var _ = Describe("QuantizeCoreFractionUp", func() {
	It("returns the input unchanged when no values are allowed", func() {
		Expect(QuantizeCoreFractionUp(37, nil)).To(Equal(37))
	})

	It("snaps up to the smallest allowed value >= raw", func() {
		Expect(QuantizeCoreFractionUp(35, fractions(25, 50, 75, 99))).To(Equal(50))
	})

	It("keeps an exact match", func() {
		Expect(QuantizeCoreFractionUp(50, fractions(25, 50, 75))).To(Equal(50))
	})

	It("returns the max when raw exceeds every allowed value", func() {
		Expect(QuantizeCoreFractionUp(80, fractions(25, 50))).To(Equal(50))
	})

	It("works with unsorted input", func() {
		Expect(QuantizeCoreFractionUp(30, fractions(99, 25, 75, 50))).To(Equal(50))
	})
})

var _ = Describe("MaxAllowedCoreFraction", func() {
	It("returns 100 when the class is nil", func() {
		Expect(MaxAllowedCoreFraction(nil, 4)).To(Equal(MaxCoreFraction))
	})

	It("returns 100 when no policy matches", func() {
		Expect(MaxAllowedCoreFraction(classWith(2, 25, 50), 4)).To(Equal(MaxCoreFraction))
	})

	It("returns 100 when the matching policy lists no fractions", func() {
		Expect(MaxAllowedCoreFraction(classWith(8), 4)).To(Equal(MaxCoreFraction))
	})

	It("returns the largest allowed fraction", func() {
		Expect(MaxAllowedCoreFraction(classWith(8, 25, 75, 50), 4)).To(Equal(75))
	})
})

var _ = Describe("AutoCoreFractions", func() {
	It("falls back to the default grid when the class is nil", func() {
		Expect(AutoCoreFractions(nil, 4)).To(Equal(DefaultAutoCoreFractionSteps))
	})

	It("falls back to the default grid when no range matches", func() {
		Expect(AutoCoreFractions(classWith(2, 25, 50), 4)).To(Equal(DefaultAutoCoreFractionSteps))
	})

	It("falls back to the default grid when the matching policy lists no fractions", func() {
		Expect(AutoCoreFractions(classWith(8), 4)).To(Equal(DefaultAutoCoreFractionSteps))
	})

	It("does not alias the default grid", func() {
		steps := AutoCoreFractions(nil, 4)
		steps[0] = 42
		Expect(DefaultAutoCoreFractionSteps[0]).To(Equal(v1alpha2.CoreFractionValue(5)))
	})

	It("returns the steps sorted, with 100% dropped", func() {
		Expect(AutoCoreFractions(classWith(8, 100, 25, 75, 50), 4)).To(Equal(fractions(25, 50, 75)))
	})

	It("leaves no step for a policy offering only 100%", func() {
		Expect(AutoCoreFractions(classWith(8, 100), 4)).To(BeEmpty())
	})

	It("keeps 99% and drops 100% when both are allowed", func() {
		Expect(AutoCoreFractions(classWith(8, 99, 100), 4)).To(Equal(fractions(99)))
	})

	It("deduplicates repeated fractions", func() {
		Expect(AutoCoreFractions(classWith(8, 25, 25, 50), 4)).To(Equal(fractions(25, 50)))
	})
})

var _ = Describe("CanAutoscaleCoreFraction", func() {
	policyWith := func(fractions ...v1alpha2.CoreFractionValue) *v1alpha2.SizingPolicy {
		return &v1alpha2.SizingPolicy{
			Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: 8},
			CoreFractions: fractions,
		}
	}

	It("allows a policy with two usable steps", func() {
		Expect(CanAutoscaleCoreFraction(policyWith(25, 50))).To(BeTrue())
	})

	It("allows a policy that constrains no fraction", func() {
		Expect(CanAutoscaleCoreFraction(policyWith())).To(BeTrue())
		Expect(CanAutoscaleCoreFraction(nil)).To(BeTrue())
	})

	It("rejects a policy with a single fraction", func() {
		Expect(CanAutoscaleCoreFraction(policyWith(50))).To(BeFalse())
	})

	It("rejects a policy whose only alternative is the dropped 100%", func() {
		Expect(CanAutoscaleCoreFraction(policyWith(50, 100))).To(BeFalse())
	})

	It("rejects a policy offering only 100%", func() {
		Expect(CanAutoscaleCoreFraction(policyWith(100))).To(BeFalse())
	})
})

var _ = Describe("SeedAutoCoreFraction", func() {
	It("returns the seed target when no policy matches", func() {
		// The default grid contains 10% itself.
		Expect(SeedAutoCoreFraction(nil, 4)).To(Equal(SeedAutoCoreFractionTarget))
	})

	It("returns the step closest to the seed target", func() {
		// Steps [25,50,75] (100% is dropped); 25 is the closest to 10.
		Expect(SeedAutoCoreFraction(classWith(8, 100, 25, 75, 50), 4)).To(Equal(25))
	})

	It("prefers the lower step on a tie", func() {
		// Steps [5,15]; both are 5 away from 10, so the VM starts small.
		Expect(SeedAutoCoreFraction(classWith(8, 5, 15), 4)).To(Equal(5))
	})

	It("falls back to the seed target when the policy leaves no step", func() {
		// A policy offering only 100% cannot autoscale at all and is rejected by the
		// webhooks; a VM that ended up on it anyway seeds at the 10% target.
		Expect(SeedAutoCoreFraction(classWith(8, 100), 4)).To(Equal(SeedAutoCoreFractionTarget))
	})
})

var _ = Describe("EffectiveCoreFraction", func() {
	autoVM := func(cores int, autoStatus string) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{CPU: v1alpha2.CPUSpec{Cores: cores, CoreFraction: v1alpha2.CoreFractionAuto}},
		}
		SetRecommendedCoreFraction(vm, autoStatus)
		return vm
	}

	It("returns the spec value verbatim when not auto", func() {
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{CPU: v1alpha2.CPUSpec{Cores: 4, CoreFraction: "50%"}}}
		Expect(EffectiveCoreFraction(vm, nil)).To(Equal("50%"))
	})

	It("uses status.recommendedResources.cpu.coreFraction when auto and it is set", func() {
		Expect(EffectiveCoreFraction(autoVM(4, "60%"), classWith(8, 25, 50, 100))).To(Equal("60%"))
	})

	It("falls back to the autoscaling seed when auto and status is empty", func() {
		// Steps [25,50,75]; the seed is the one closest to 10%.
		Expect(EffectiveCoreFraction(autoVM(4, ""), classWith(8, 25, 75, 50))).To(Equal("25%"))
	})

	It("falls back to the default-grid seed when auto with no policy", func() {
		Expect(EffectiveCoreFraction(autoVM(4, ""), nil)).To(Equal("10%"))
	})
})

var _ = Describe("ParsePercent", func() {
	It("parses a percentage string", func() {
		v, err := ParsePercent("42%")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal(42))
	})

	It("parses a bare number", func() {
		v, err := ParsePercent("42")
		Expect(err).ToNot(HaveOccurred())
		Expect(v).To(Equal(42))
	})

	It("errors on a non-numeric value", func() {
		_, err := ParsePercent("auto")
		Expect(err).To(HaveOccurred())
	})
})
