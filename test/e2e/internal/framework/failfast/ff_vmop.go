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

package failfast

import (
	"fmt"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

// vmopDeadEndReasons are Completed-condition reasons a Pending VMOP cannot
// recover from without the spec changing something: the VM will not appear on
// its own, its runPolicy and live-migration policy do not change, and hotplug
// disks do not become shared. Capacity-like reasons (QuotaExceeded,
// OtherMigrationInProgress, NotReadyToBeExecuted) heal on their own and are
// deliberately absent.
var vmopDeadEndReasons = map[string]struct{}{
	vmopcondition.ReasonVirtualMachineNotFound.String():              {},
	vmopcondition.ReasonNotApplicableForRunPolicy.String():           {},
	vmopcondition.ReasonNotApplicableForVMPhase.String():             {},
	vmopcondition.ReasonNotApplicableForLiveMigrationPolicy.String(): {},
	vmopcondition.ReasonHotplugDisksNotShared.String():               {},
}

// VMOPFailed fails the spec when a VMOP of the namespace settles in the Failed
// phase. The failure often never surfaces on the object the spec is waiting
// on: after a failed migration the run policy restarts the machine, the
// recreated VMI carries no migration state, and the VM parks Running with
// clean conditions - so a VM-side wait burns its whole timeout while the VMOP
// has been terminal for minutes. The grace period lets a wait on the VMOP
// itself classify known KubeVirt flakes into a Skip (ending the spec and the
// rule with it) before the rule fires.
//
// A spec that deliberately drives a VMOP into the Failed phase (a stop that
// must be rejected, a cancelled migration) registers it via ExpectFailure;
// suites built around failing migrations disable the rule wholesale with
// TolerateFailedMigrations.
func VMOPFailed(vmops Client[*v1alpha2.VirtualMachineOperation], namespace string) FailFast {
	return New("VirtualMachineOperation "+namespace+"/", vmops, func(vmop *v1alpha2.VirtualMachineOperation) *Finding {
		if vmop.Status.Phase != v1alpha2.VMOPPhaseFailed {
			return nil
		}
		cond, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
		return &Finding{
			Message: fmt.Sprintf("is Failed: reason: %s, message: %s", cond.Reason, cond.Message),
			Grace:   defaultGrace,
		}
	})
}

// VMOPStuckPending fails the spec when a VMOP of the namespace is parked in
// the Pending phase for a reason that cannot heal without the spec's
// intervention.
func VMOPStuckPending(vmops Client[*v1alpha2.VirtualMachineOperation], namespace string) FailFast {
	return New("VirtualMachineOperation "+namespace+"/", vmops, func(vmop *v1alpha2.VirtualMachineOperation) *Finding {
		if vmop.Status.Phase != v1alpha2.VMOPPhasePending {
			return nil
		}
		cond, found := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
		if !found {
			return nil
		}
		if _, deadEnd := vmopDeadEndReasons[cond.Reason]; !deadEnd {
			return nil
		}
		return &Finding{
			Message: fmt.Sprintf("stays Pending with the dead-end reason %s: %s", cond.Reason, cond.Message),
			Grace:   defaultGrace,
		}
	})
}
