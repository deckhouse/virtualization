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

package vmop

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

func IsInProgressOrPending(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == "" || vmop.Status.Phase == v1alpha2.VMOPPhasePending || vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress)
}

func IsFinished(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == v1alpha2.VMOPPhaseFailed || vmop.Status.Phase == v1alpha2.VMOPPhaseCompleted)
}

func IsTerminating(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == v1alpha2.VMOPPhaseTerminating || !vmop.GetDeletionTimestamp().IsZero())
}

func IsMigration(vmop *v1alpha2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Spec.Type == v1alpha2.VMOPTypeMigrate || vmop.Spec.Type == v1alpha2.VMOPTypeEvict)
}

func InProgressOrPendingExists(vmops []v1alpha2.VirtualMachineOperation) bool {
	for _, vmop := range vmops {
		if IsInProgressOrPending(&vmop) {
			return true
		}
	}
	return false
}

func IsOperationInProgress(vmop *v1alpha2.VirtualMachineOperation) bool {
	sent, _ := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	return sent.Status == metav1.ConditionTrue && !IsFinished(vmop)
}
