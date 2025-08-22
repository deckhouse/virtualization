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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IsInProgressOrPending(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == "" || vmop.Status.Phase == virtv2.VMOPPhasePending || vmop.Status.Phase == virtv2.VMOPPhaseInProgress)
}

func IsFinished(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == virtv2.VMOPPhaseFailed || vmop.Status.Phase == virtv2.VMOPPhaseCompleted)
}

func IsTerminating(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == virtv2.VMOPPhaseTerminating || !vmop.GetDeletionTimestamp().IsZero())
}

func IsMigration(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Spec.Type == virtv2.VMOPTypeMigrate || vmop.Spec.Type == virtv2.VMOPTypeEvict)
}

func InProgressOrPendingExists(vmops []virtv2.VirtualMachineOperation) bool {
	for _, vmop := range vmops {
		if IsInProgressOrPending(&vmop) {
			return true
		}
	}
	return false
}
