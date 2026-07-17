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

package service

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// vm builds an autoscaled VM whose current effective coreFraction is carried in
// status.recommendedResources.cpu.coreFraction. With cores=4 each coreFraction percent maps to 40m.
func vm(cores int, current string) *v1alpha2.VirtualMachine {
	out := &v1alpha2.VirtualMachine{
		Spec: v1alpha2.VirtualMachineSpec{
			CPU: v1alpha2.CPUSpec{Cores: cores, CoreFraction: v1alpha2.CoreFractionAuto},
		},
	}
	sizingpolicy.SetRecommendedCoreFraction(out, current)
	return out
}

func classWithFractions(max int, fractions ...int) *v1alpha2.VirtualMachineClass {
	cf := make([]v1alpha2.CoreFractionValue, len(fractions))
	for i, f := range fractions {
		cf[i] = v1alpha2.CoreFractionValue(f)
	}
	return &v1alpha2.VirtualMachineClass{
		Spec: v1alpha2.VirtualMachineClassSpec{
			SizingPolicies: []v1alpha2.SizingPolicy{
				{
					Cores:         &v1alpha2.SizingPolicyCores{Min: 1, Max: max},
					CoreFractions: cf,
				},
			},
		},
	}
}

var _ = Describe("CoreFractionService", func() {
	s := NewCoreFractionService()

	Context("anti-flapping via the recommended range", func() {
		It("holds still while the current request is within [LowerBound, UpperBound]", func() {
			// current 50% -> 2000m, inside [1600, 2400]; the target is ignored.
			d, err := s.Calculate(vm(4, "50%"), nil, Recommendation{TargetMilli: 3000, LowerMilli: 1600, UpperMilli: 2400})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.Direction).To(Equal(DirectionNone))
			Expect(d.CurrentCoreFraction).To(Equal(50))
		})

		It("does not act when the recommendation carries no bounds", func() {
			d, err := s.Calculate(vm(4, "10%"), nil, Recommendation{TargetMilli: 4000})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.Direction).To(Equal(DirectionNone))
		})
	})

	Context("scaling up when below the lower bound", func() {
		It("snaps up to the default grid step without a policy", func() {
			// current 10% -> 400m < 1000 lower; target 1370m -> ceil(1370/40)=35% -> grid 40%.
			d, err := s.Calculate(vm(4, "10%"), nil, Recommendation{TargetMilli: 1370, LowerMilli: 1000, UpperMilli: 2000})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(40))
			Expect(d.Direction).To(Equal(DirectionUp))
		})

		It("clamps an oversized target down to the autoscaling ceiling (99)", func() {
			// current 50% -> 2000m < 3000 lower; target 8000m -> raw 200 -> clamp 99.
			d, err := s.Calculate(vm(4, "50%"), nil, Recommendation{TargetMilli: 8000, LowerMilli: 3000, UpperMilli: 4000})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(99))
			Expect(d.Direction).To(Equal(DirectionUp))
		})

		It("snaps the raw fraction up to the nearest allowed policy value", func() {
			class := classWithFractions(8, 25, 50, 75, 100)
			// current 25% -> 1000m < 1600 lower; target 1370m -> raw 35% -> allowed 50%.
			d, err := s.Calculate(vm(4, "25%"), class, Recommendation{TargetMilli: 1370, LowerMilli: 1600, UpperMilli: 2400})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(50))
			Expect(d.Direction).To(Equal(DirectionUp))
		})

		It("holds at the largest allowed value when the target exceeds it", func() {
			class := classWithFractions(8, 25, 50)
			// current 25% -> 1000m < 1600 lower; target 3000m -> raw 75% -> capped at the max allowed 50%.
			d, err := s.Calculate(vm(4, "25%"), class, Recommendation{TargetMilli: 3000, LowerMilli: 1600, UpperMilli: 2400})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(50))
			Expect(d.Direction).To(Equal(DirectionUp))
		})

		It("never picks a policy's 100%, holding at the step below it", func() {
			class := classWithFractions(8, 25, 50, 100)
			// current 25% -> 1000m < 1600 lower; target 8000m -> raw 99%; 100% is dropped
			// from the steps, so the top the autoscaler may reach is the policy's 50%.
			d, err := s.Calculate(vm(4, "25%"), class, Recommendation{TargetMilli: 8000, LowerMilli: 1600, UpperMilli: 2400})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(50))
			Expect(d.Direction).To(Equal(DirectionUp))
		})

		It("reaches 99% only when the policy lists it", func() {
			class := classWithFractions(8, 25, 50, 99, 100)
			// current 25% -> 1000m < 1600 lower; target 8000m -> raw 99% -> the policy's 99%.
			d, err := s.Calculate(vm(4, "25%"), class, Recommendation{TargetMilli: 8000, LowerMilli: 1600, UpperMilli: 2400})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(99))
			Expect(d.Direction).To(Equal(DirectionUp))
		})

		It("ignores a policy whose cores range does not match", func() {
			class := classWithFractions(2, 25, 50) // VM has 4 cores, out of range.
			// raw 35% falls through to the default grid -> 40%.
			d, err := s.Calculate(vm(4, "25%"), class, Recommendation{TargetMilli: 1370, LowerMilli: 1600, UpperMilli: 2400})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(40))
		})
	})

	Context("scaling down when above the upper bound", func() {
		It("moves to the target below the current value", func() {
			// current 90% -> 3600m > 2000 upper; target 1400m -> raw 35% -> grid 40%.
			d, err := s.Calculate(vm(4, "90%"), nil, Recommendation{TargetMilli: 1400, LowerMilli: 1000, UpperMilli: 2000})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(40))
			Expect(d.Direction).To(Equal(DirectionDown))
		})

		It("clamps a tiny target up to the lowest grid step", func() {
			// current 50% -> 2000m > 100 upper; target 1m -> raw 1% -> lowest grid step 5%.
			d, err := s.Calculate(vm(4, "50%"), nil, Recommendation{TargetMilli: 1, LowerMilli: 40, UpperMilli: 100})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.DesiredCoreFraction).To(Equal(5))
			Expect(d.Direction).To(Equal(DirectionDown))
		})
	})

	Context("edge cases", func() {
		It("falls back to the autoscaling seed when the desired value is empty", func() {
			// No policy -> default-grid seed of 10% -> 400m, inside [300, 500] -> no move.
			d, err := s.Calculate(vm(4, ""), nil, Recommendation{TargetMilli: 400, LowerMilli: 300, UpperMilli: 500})
			Expect(err).ToNot(HaveOccurred())
			Expect(d.CurrentCoreFraction).To(Equal(10))
			Expect(d.Direction).To(Equal(DirectionNone))
		})

		It("errors on a non-positive core count", func() {
			_, err := s.Calculate(vm(0, "50%"), nil, Recommendation{TargetMilli: 1000, LowerMilli: 1, UpperMilli: 2})
			Expect(err).To(HaveOccurred())
		})

		It("errors on an unparsable coreFraction", func() {
			_, err := s.Calculate(vm(4, "bogus"), nil, Recommendation{TargetMilli: 1000, LowerMilli: 1, UpperMilli: 2})
			Expect(err).To(HaveOccurred())
		})
	})
})
