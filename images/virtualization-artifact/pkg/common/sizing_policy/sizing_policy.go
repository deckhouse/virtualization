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

// SeedAutoCoreFractionTarget is the coreFraction the autoscaler aims for before the
// recommender has any data. It is deliberately low: a fresh VM is assumed idle, and
// climbing up on demand is cheaper than holding requests nobody uses.
const SeedAutoCoreFractionTarget = 10

// DefaultAutoCoreFractionSteps is the grid the autoscaler walks when the sizing policy
// puts no constraint on coreFraction. Without it the autoscaler would be free to pick
// any percentage in [1,99] and would nudge the VM on every single-percent drift; the
// grid is coarse enough to keep changes rare and fine enough to follow the load. It
// tops out at MaxAutoCoreFraction rather than 100% for the QoS reason above.
var DefaultAutoCoreFractionSteps = []v1alpha2.CoreFractionValue{5, 10, 15, 20, 30, 40, 50, 60, 70, 80, 90, MaxAutoCoreFraction}

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
// greater than or equal to it. When raw exceeds every allowed value, the largest
// allowed value is returned. With no allowed values the input is returned unchanged;
// callers get their steps from AutoCoreFractions, which never returns an empty grid.
func QuantizeCoreFractionUp(raw int, allowed []v1alpha2.CoreFractionValue) int {
	if len(allowed) == 0 {
		return raw
	}

	sorted := make([]int, len(allowed))
	for i, a := range allowed {
		sorted[i] = int(a)
	}
	slices.Sort(sorted)

	for _, v := range sorted {
		if v >= raw {
			return v
		}
	}

	// raw is above every allowed value: fall back to the largest.
	return sorted[len(sorted)-1]
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

// MinAutoCoreFractionSteps is the number of steps autoscaling needs to make sense: with
// a single step the value can never change, so "Auto" would be an elaborate way of
// writing that one percentage.
const MinAutoCoreFractionSteps = 2

// AutoCoreFractions returns the coreFraction steps the autoscaler may quantize to for
// a VM with the given cores, in ascending order. See AutoCoreFractionSteps for how the
// steps are derived from the sizing policy.
func AutoCoreFractions(class *v1alpha2.VirtualMachineClass, cores int) []v1alpha2.CoreFractionValue {
	return AutoCoreFractionSteps(MatchSizingPolicy(class, cores))
}

// AutoCoreFractionSteps returns the coreFraction steps of a single sizing policy, in
// ascending order: its own steps without 100%. 100% is dropped rather than lowered to 99%:
// requests would equal limits, making the pod Guaranteed and breaking in-place resize
// (see MaxAutoCoreFraction), and silently turning an allowed 100% into a 99% the policy
// never listed would be a value the administrator did not permit. A policy with no
// coreFractions (or no policy at all) puts no constraint on the fraction, so the
// autoscaler walks DefaultAutoCoreFractionSteps instead of the whole [1,99] range.
//
// The result may hold fewer than MinAutoCoreFractionSteps steps — such a policy cannot
// autoscale at all, and both webhooks reject "Auto" against it (see
// CanAutoscaleCoreFraction).
func AutoCoreFractionSteps(sp *v1alpha2.SizingPolicy) []v1alpha2.CoreFractionValue {
	if sp == nil || len(sp.CoreFractions) == 0 {
		return slices.Clone(DefaultAutoCoreFractionSteps)
	}
	seen := make(map[int]struct{}, len(sp.CoreFractions))
	steps := make([]v1alpha2.CoreFractionValue, 0, len(sp.CoreFractions))
	for _, f := range sp.CoreFractions {
		v := int(f)
		if v >= MaxCoreFraction {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		steps = append(steps, v1alpha2.CoreFractionValue(v))
	}
	slices.SortFunc(steps, func(a, b v1alpha2.CoreFractionValue) int { return int(a) - int(b) })
	return steps
}

// CanAutoscaleCoreFraction reports whether a sizing policy leaves the autoscaler room to
// move: at least MinAutoCoreFractionSteps steps below 100%. A policy listing a single
// fraction — or a single one plus the dropped 100% — pins the VM to that value, so
// "Auto" is rejected for it instead of quietly behaving like a fixed percentage.
func CanAutoscaleCoreFraction(sp *v1alpha2.SizingPolicy) bool {
	return len(AutoCoreFractionSteps(sp)) >= MinAutoCoreFractionSteps
}

// SeedAutoCoreFraction returns the coreFraction the autoscaler seeds a VM with before
// the recommender has data: the step closest to SeedAutoCoreFractionTarget (10%), ties
// going to the lower one. A VM with no recommendation yet is assumed idle, so it starts
// small and climbs once the recommender has something to say. Steps come from
// AutoCoreFractions, so an unconstrained VM seeds at exactly 10%. A policy that leaves no
// step at all cannot be used with "Auto" (the webhooks reject it); should such a VM exist
// anyway — the class was edited under it — the seed falls back to the 10% target.
func SeedAutoCoreFraction(class *v1alpha2.VirtualMachineClass, cores int) int {
	steps := AutoCoreFractions(class, cores)
	if len(steps) == 0 {
		return SeedAutoCoreFractionTarget
	}

	seed := int(steps[0])
	for _, s := range steps[1:] {
		if abs(int(s)-SeedAutoCoreFractionTarget) < abs(seed-SeedAutoCoreFractionTarget) {
			seed = int(s)
		}
	}
	return seed
}

// EffectiveCoreFraction resolves the coreFraction to apply to a VM. When
// spec.cpu.coreFraction is a plain percentage it is returned as is. When it is
// "Auto", the value driven by the autoscaler
// (status.recommendedResources.cpu.coreFraction) is used; until the autoscaler has set
// it, the fallback is the autoscaling seed. The result is always a "N%" string.
func EffectiveCoreFraction(vm *v1alpha2.VirtualMachine, class *v1alpha2.VirtualMachineClass) string {
	if vm.Spec.CPU.CoreFraction != v1alpha2.CoreFractionAuto {
		return vm.Spec.CPU.CoreFraction
	}
	if recommended := RecommendedCoreFraction(vm); recommended != "" {
		return recommended
	}
	return fmt.Sprintf("%d%%", SeedAutoCoreFraction(class, vm.Spec.CPU.Cores))
}

// RecommendedCoreFraction returns the coreFraction the autoscaler asks for
// (status.recommendedResources.cpu.coreFraction), or an empty string when it has not set
// one.
func RecommendedCoreFraction(vm *v1alpha2.VirtualMachine) string {
	if vm == nil || vm.Status.RecommendedResources == nil {
		return ""
	}
	return vm.Status.RecommendedResources.CPU.CoreFraction
}

// SetRecommendedCoreFraction publishes the coreFraction the autoscaler asks for. An empty
// value drops the whole recommendedResources section, so a VM that left "Auto" carries no
// stale recommendation.
func SetRecommendedCoreFraction(vm *v1alpha2.VirtualMachine, coreFraction string) {
	if coreFraction == "" {
		vm.Status.RecommendedResources = nil
		return
	}
	if vm.Status.RecommendedResources == nil {
		vm.Status.RecommendedResources = &v1alpha2.RecommendedResourcesStatus{}
	}
	vm.Status.RecommendedResources.CPU.CoreFraction = coreFraction
}

// ParsePercent parses a "N%" string into its integer percentage.
func ParsePercent(s string) (int, error) {
	return strconv.Atoi(strings.TrimSuffix(s, "%"))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
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
