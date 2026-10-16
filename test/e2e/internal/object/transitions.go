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

package object

import (
	"time"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// HasPhaseTransitionAfter reports whether the VirtualMachine's phase-transition
// history records a transition into phase with a timestamp strictly after passed timestamp.
func HasPhaseTransitionAfter(vm *v1alpha2.VirtualMachine, phase v1alpha2.MachinePhase, timestamp time.Time) bool {
	if vm.Status.Stats == nil {
		return false
	}
	transition := FindPhaseLastTransition(vm, phase)
	if transition == nil {
		return false
	}
	return transition.Timestamp.After(timestamp)
}

// FindPhaseLastTransition returns the more recent transition with the matched phase.
func FindPhaseLastTransition(vm *v1alpha2.VirtualMachine, phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachinePhaseTransitionTimestamp {
	if vm.Status.Stats == nil {
		return nil
	}
	count := len(vm.Status.Stats.PhasesTransitions)
	for i := count - 1; i >= 0; i-- {
		transition := vm.Status.Stats.PhasesTransitions[i]
		if transition.Phase == phase {
			return &transition
		}
	}
	return nil
}
