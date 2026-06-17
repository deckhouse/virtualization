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

package vm

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

// BeRunning reports the VirtualMachine has reached the Running phase. Intended
// for use with [Observer.WaitFor].
func BeRunning() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		return vm.Status.Phase == v1alpha2.MachineRunning, nil
	}
}

// BeAgentReady reports the VirtualMachine's guest agent is ready, i.e. the
// AgentReady condition is present with Status=True. Intended for use with
// [Observer.WaitFor].
func BeAgentReady() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond := findCondition(vm.Status.Conditions, vmcondition.TypeAgentReady.String())
		if cond == nil {
			return false, nil
		}
		return cond.Status == metav1.ConditionTrue, nil
	}
}

// BeFailed reports an invariant violation when the VirtualMachine has entered
// the terminal Degraded phase. Intended for use with [Observer.Never].
func BeFailed() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		if vm.Status.Phase == v1alpha2.MachineDegraded {
			return true, fmt.Errorf("VirtualMachine entered Degraded phase")
		}
		return false, nil
	}
}

func findCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}
