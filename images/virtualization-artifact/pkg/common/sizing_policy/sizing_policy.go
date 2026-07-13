/*
Copyright 2025 Flant JSC

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
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func FormatCoreFractionValues(cf []v1alpha2.CoreFractionValue) []string {
	result := make([]string, len(cf))
	for i, v := range cf {
		result[i] = fmt.Sprintf("%d%%", v)
	}
	return result
}

// MinCoreFraction and MaxCoreFraction bound the coreFraction percentage, matching
// the CoreFractionValue kubebuilder validation (1..100).
const (
	MinCoreFraction = 1
	MaxCoreFraction = 100
)

// MaxAutoCoreFraction is the ceiling the autoscaler may pick. It stays strictly
// below MaxCoreFraction on purpose: at 100% the launcher pod's CPU requests equal
// its limits, so the pod is Guaranteed, while any lower value keeps it Burstable.
// Kubernetes forbids an in-place resize from changing a pod's QoS class, so the
// whole autoscaling range must live in a single class — Burstable, i.e. below 100%.
const MaxAutoCoreFraction = 99

// MatchSizingPolicy returns the sizing policy whose cores range contains the given
// number of cores, or nil if the class has none or no range matches. The returned
// policy is a deep copy and safe to retain.
func MatchSizingPolicy(class *v1alpha2.VirtualMachineClass, cores int) *v1alpha2.SizingPolicy {
	if class == nil {
		return nil
	}
	for _, sp := range class.Spec.SizingPolicies {
		if sp.Cores == nil {
			continue
		}
		if cores >= sp.Cores.Min && cores <= sp.Cores.Max {
			return sp.DeepCopy()
		}
	}
	return nil
}

// NeededCoreFraction returns the smallest coreFraction percentage whose CPU
// requests (cores * fraction%) cover targetMilliCPU. cores*1000m equals 100%, so
// the raw percentage is targetMilliCPU / (cores*10), rounded up. The result is
// clamped to [MinCoreFraction, MaxAutoCoreFraction]: this feeds the autoscaler, so
// it must never reach 100% and flip the pod to Guaranteed (see MaxAutoCoreFraction).
// cores must be positive.
func NeededCoreFraction(cores int, targetMilliCPU int64) int {
	if cores <= 0 {
		return MaxAutoCoreFraction
	}
	denom := int64(cores) * 10
	fraction := int((targetMilliCPU + denom - 1) / denom) // ceil division
	return clamp(fraction, MinCoreFraction, MaxAutoCoreFraction)
}

// QuantizeCoreFractionUp snaps raw up to the smallest allowed coreFraction that is
// greater than or equal to it. When raw exceeds every allowed value, it returns
// the largest allowed value and exceedsMax=true. With no allowed values the input
// is returned unchanged (free choice within [1,100]).
func QuantizeCoreFractionUp(raw int, allowed []v1alpha2.CoreFractionValue) (val int, exceedsMax bool) {
	if len(allowed) == 0 {
		return raw, false
	}

	sorted := make([]int, len(allowed))
	for i, a := range allowed {
		sorted[i] = int(a)
	}
	slices.Sort(sorted)

	for _, v := range sorted {
		if v >= raw {
			return v, false
		}
	}

	// raw is above every allowed value: fall back to the largest.
	return sorted[len(sorted)-1], true
}

// MaxAllowedCoreFraction returns the largest coreFraction percentage allowed by
// the sizing policy matching cores, or MaxCoreFraction (100) when there is no
// matching policy or it lists no fractions.
func MaxAllowedCoreFraction(class *v1alpha2.VirtualMachineClass, cores int) int {
	sp := MatchSizingPolicy(class, cores)
	if sp == nil || len(sp.CoreFractions) == 0 {
		return MaxCoreFraction
	}
	max := MinCoreFraction
	for _, f := range sp.CoreFractions {
		if int(f) > max {
			max = int(f)
		}
	}
	return max
}

// AutoCoreFractions returns the coreFraction steps the autoscaler may quantize to
// for a VM with the given cores, in ascending order: the matching sizing policy's
// steps below 100% (100% is excluded because it makes the pod Guaranteed and breaks
// in-place resize, see MaxAutoCoreFraction). The bool reports whether a sizing
// policy matched; when false the autoscaler is free to pick any value within
// [MinCoreFraction, MaxAutoCoreFraction]. A matching policy that lists only 100%
// yields an empty slice with true — the VM cannot be autoscaled by coreFraction.
func AutoCoreFractions(class *v1alpha2.VirtualMachineClass, cores int) ([]v1alpha2.CoreFractionValue, bool) {
	sp := MatchSizingPolicy(class, cores)
	if sp == nil {
		return nil, false
	}
	steps := make([]v1alpha2.CoreFractionValue, 0, len(sp.CoreFractions))
	for _, f := range sp.CoreFractions {
		if int(f) < MaxCoreFraction {
			steps = append(steps, f)
		}
	}
	slices.SortFunc(steps, func(a, b v1alpha2.CoreFractionValue) int { return int(a) - int(b) })
	return steps, true
}

// SeedAutoCoreFraction returns the coreFraction the autoscaler seeds a VM with
// before the recommender has data: a middle value among the allowed Burstable steps
// (deliberately not the maximum), or the midpoint of [MinCoreFraction,
// MaxAutoCoreFraction] when no sizing policy constrains the VM. A policy that offers
// no Burstable step falls back to the ceiling; validation should keep auto off such
// policies.
func SeedAutoCoreFraction(class *v1alpha2.VirtualMachineClass, cores int) int {
	steps, hasPolicy := AutoCoreFractions(class, cores)
	if !hasPolicy {
		return (MinCoreFraction + MaxAutoCoreFraction) / 2
	}
	if len(steps) == 0 {
		return MaxAutoCoreFraction
	}
	return int(steps[len(steps)/2])
}

// EffectiveCoreFraction resolves the coreFraction to apply to a VM. When
// spec.cpu.coreFraction is a plain percentage it is returned as is. When it is
// "auto", the value driven by the autoscaler (status.autoCoreFraction) is used;
// until the autoscaler has set it, the fallback is the autoscaling seed. The result
// is always a "N%" string.
func EffectiveCoreFraction(vm *v1alpha2.VirtualMachine, class *v1alpha2.VirtualMachineClass) string {
	if vm.Spec.CPU.CoreFraction != v1alpha2.CoreFractionAuto {
		return vm.Spec.CPU.CoreFraction
	}
	if vm.Status.AutoCoreFraction != "" {
		return vm.Status.AutoCoreFraction
	}
	return fmt.Sprintf("%d%%", SeedAutoCoreFraction(class, vm.Spec.CPU.Cores))
}

// ParsePercent parses a "N%" string into its integer percentage.
func ParsePercent(s string) (int, error) {
	return strconv.Atoi(strings.TrimSuffix(s, "%"))
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
