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
	"fmt"

	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Direction is the way a VM's coreFraction should move.
type Direction int

const (
	DirectionNone Direction = iota
	DirectionUp
	DirectionDown
)

func (d Direction) String() string {
	switch d {
	case DirectionUp:
		return "up"
	case DirectionDown:
		return "down"
	default:
		return "none"
	}
}

// Recommendation is the VPA CPU recommendation for the compute container, in millicores.
type Recommendation struct {
	TargetMilli int64
	LowerMilli  int64
	UpperMilli  int64
}

type Decision struct {
	CurrentCoreFraction int
	DesiredCoreFraction int
	Direction           Direction
	// ExceedsPolicyMax is set when the target is above every allowed coreFraction:
	// the VM cannot be satisfied by coreFraction alone.
	ExceedsPolicyMax bool
}

// CoreFractionService derives the desired coreFraction for an autoscaled VM from a
// VPA recommendation. It inverts the CPU target into the coreFraction whose requests
// cover it (capped below 100% to keep the pod Burstable) and snaps it to the sizing
// policy grid. To avoid flapping it acts only when the current CPU request leaves the
// recommended range [LowerBound, UpperBound].
type CoreFractionService struct{}

func NewCoreFractionService() *CoreFractionService {
	return &CoreFractionService{}
}

func (s *CoreFractionService) Calculate(vm *v1alpha2.VirtualMachine, class *v1alpha2.VirtualMachineClass, rec Recommendation) (Decision, error) {
	cores := vm.Spec.CPU.Cores
	if cores <= 0 {
		return Decision{}, fmt.Errorf("virtual machine %s/%s has a non-positive core count", vm.GetNamespace(), vm.GetName())
	}

	current, err := sizingpolicy.ParsePercent(sizingpolicy.EffectiveCoreFraction(vm, class))
	if err != nil {
		return Decision{}, fmt.Errorf("parse effective coreFraction of %s/%s: %w", vm.GetNamespace(), vm.GetName(), err)
	}

	// The current CPU request is cores*10m per coreFraction percent. Hold still while
	// it stays inside the recommended range.
	currentReqMilli := int64(cores) * 10 * int64(current)
	belowLower := rec.LowerMilli > 0 && currentReqMilli < rec.LowerMilli
	aboveUpper := rec.UpperMilli > 0 && currentReqMilli > rec.UpperMilli
	if !belowLower && !aboveUpper {
		return Decision{CurrentCoreFraction: current, DesiredCoreFraction: current, Direction: DirectionNone}, nil
	}

	raw := sizingpolicy.NeededCoreFraction(cores, rec.TargetMilli)
	steps, hasPolicy := sizingpolicy.AutoCoreFractions(class, cores)

	desired := raw
	var exceedsMax bool
	switch {
	case !hasPolicy:
		// Free choice; raw stands.
	case len(steps) == 0:
		// Only 100% is allowed: coreFraction cannot autoscale this VM. Sit at the ceiling.
		desired = sizingpolicy.MaxAutoCoreFraction
		exceedsMax = true
	default:
		desired, exceedsMax = sizingpolicy.QuantizeCoreFractionUp(raw, steps)
	}

	d := Decision{CurrentCoreFraction: current, DesiredCoreFraction: desired, ExceedsPolicyMax: exceedsMax}
	switch {
	case desired > current:
		d.Direction = DirectionUp
	case desired < current:
		d.Direction = DirectionDown
	default:
		d.Direction = DirectionNone
	}

	return d, nil
}
