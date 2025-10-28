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

package vmsop

import (
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func IsInProgressOrPending(vmsop *v1alpha2.VirtualMachineSnapshotOperation) bool {
	return vmsop != nil && (vmsop.Status.Phase == "" || vmsop.Status.Phase == v1alpha2.VMSOPPhasePending || vmsop.Status.Phase == v1alpha2.VMSOPPhaseInProgress)
}

func IsFinished(vmsop *v1alpha2.VirtualMachineSnapshotOperation) bool {
	return vmsop != nil && (vmsop.Status.Phase == v1alpha2.VMSOPPhaseFailed || vmsop.Status.Phase == v1alpha2.VMSOPPhaseCompleted)
}

func IsTerminating(vmsop *v1alpha2.VirtualMachineSnapshotOperation) bool {
	return vmsop != nil && (vmsop.Status.Phase == v1alpha2.VMSOPPhaseTerminating || !vmsop.GetDeletionTimestamp().IsZero())
}

func InProgressOrPendingExists(vmsops []v1alpha2.VirtualMachineSnapshotOperation) bool {
	for _, vmsop := range vmsops {
		if IsInProgressOrPending(&vmsop) {
			return true
		}
	}
	return false
}
