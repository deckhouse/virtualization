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
		val, exceeds := QuantizeCoreFractionUp(37, nil)
		Expect(val).To(Equal(37))
		Expect(exceeds).To(BeFalse())
	})

	It("snaps up to the smallest allowed value >= raw", func() {
		val, exceeds := QuantizeCoreFractionUp(35, fractions(25, 50, 75, 100))
		Expect(val).To(Equal(50))
		Expect(exceeds).To(BeFalse())
	})

	It("keeps an exact match", func() {
		val, exceeds := QuantizeCoreFractionUp(50, fractions(25, 50, 75))
		Expect(val).To(Equal(50))
		Expect(exceeds).To(BeFalse())
	})

	It("returns the max and flags when raw exceeds every allowed value", func() {
		val, exceeds := QuantizeCoreFractionUp(80, fractions(25, 50))
		Expect(val).To(Equal(50))
		Expect(exceeds).To(BeTrue())
	})

	It("works with unsorted input", func() {
		val, exceeds := QuantizeCoreFractionUp(30, fractions(100, 25, 75, 50))
		Expect(val).To(Equal(50))
		Expect(exceeds).To(BeFalse())
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
	It("reports no policy when the class is nil", func() {
		steps, hasPolicy := AutoCoreFractions(nil, 4)
		Expect(hasPolicy).To(BeFalse())
		Expect(steps).To(BeNil())
	})

	It("reports no policy when no range matches", func() {
		steps, hasPolicy := AutoCoreFractions(classWith(2, 25, 50), 4)
		Expect(hasPolicy).To(BeFalse())
		Expect(steps).To(BeNil())
	})

	It("returns the Burstable steps sorted, excluding 100%", func() {
		steps, hasPolicy := AutoCoreFractions(classWith(8, 100, 25, 75, 50), 4)
		Expect(hasPolicy).To(BeTrue())
		Expect(steps).To(Equal(fractions(25, 50, 75)))
	})

	It("returns an empty slice when the policy offers only 100%", func() {
		steps, hasPolicy := AutoCoreFractions(classWith(8, 100), 4)
		Expect(hasPolicy).To(BeTrue())
		Expect(steps).To(BeEmpty())
	})
})

var _ = Describe("SeedAutoCoreFraction", func() {
	It("returns the free-range midpoint when no policy matches", func() {
		Expect(SeedAutoCoreFraction(nil, 4)).To(Equal((MinCoreFraction + MaxAutoCoreFraction) / 2))
	})

	It("returns the middle Burstable step", func() {
		Expect(SeedAutoCoreFraction(classWith(8, 100, 25, 75, 50), 4)).To(Equal(50))
	})

	It("falls back to the ceiling when the policy offers only 100%", func() {
		Expect(SeedAutoCoreFraction(classWith(8, 100), 4)).To(Equal(MaxAutoCoreFraction))
	})
})

var _ = Describe("EffectiveCoreFraction", func() {
	autoVM := func(cores int, autoStatus string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			Spec:   v1alpha2.VirtualMachineSpec{CPU: v1alpha2.CPUSpec{Cores: cores, CoreFraction: v1alpha2.CoreFractionAuto}},
			Status: v1alpha2.VirtualMachineStatus{AutoCoreFraction: autoStatus},
		}
	}

	It("returns the spec value verbatim when not auto", func() {
		vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{CPU: v1alpha2.CPUSpec{Cores: 4, CoreFraction: "50%"}}}
		Expect(EffectiveCoreFraction(vm, nil)).To(Equal("50%"))
	})

	It("uses status.autoCoreFraction when auto and it is set", func() {
		Expect(EffectiveCoreFraction(autoVM(4, "60%"), classWith(8, 25, 50, 100))).To(Equal("60%"))
	})

	It("falls back to the autoscaling seed when auto and status is empty", func() {
		// Burstable steps [25,50,75]; seed is the middle one.
		Expect(EffectiveCoreFraction(autoVM(4, ""), classWith(8, 25, 75, 50))).To(Equal("50%"))
	})

	It("falls back to the free-range seed when auto with no policy", func() {
		Expect(EffectiveCoreFraction(autoVM(4, ""), nil)).To(Equal("50%"))
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
