/*
Copyright 2024 Flant JSC

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

package powerstate

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
)

var ErrChangesAlreadyExist = errors.New("changes already exist in the current status")

// BuildPatch creates a patch to request VM state changing via updating KVVM status.
//
// Some combinations lead to an error to not interfere with kvvm controller:
//
// current  /  desired         stop      start     restart(stop+start)
// stop                        replace   error     error
// start                       replace   error     error
// restart(stop+start)         replace   error     error
// empty                       add       add       add
func BuildPatch(vm *virtv1.VirtualMachine, changes ...virtv1.VirtualMachineStateChangeRequest) ([]byte, error) {
	jp := patch.NewJSONPatch()
	// Special case: if there's no status field at all, add one.
	newStatus := virtv1.VirtualMachineStatus{}
	if equality.Semantic.DeepEqual(vm.Status, newStatus) {
		newStatus.StateChangeRequests = changes
		jp.Append(patch.NewJSONPatchOperation(patch.PatchAddOp, "/status", newStatus))
	} else {
		verb := patch.PatchAddOp
		failOnConflict := true
		if len(changes) == 1 && changes[0].Action == virtv1.StopRequest {
			// If this is a stopRequest, replace all existing StateChangeRequests.
			failOnConflict = false
		}
		if len(vm.Status.StateChangeRequests) != 0 {
			if equality.Semantic.DeepEqual(vm.Status.StateChangeRequests, changes) {
				return nil, ErrChangesAlreadyExist
			}

			if failOnConflict {
				return nil, fmt.Errorf("unable to complete request: stop/start already underway")
			} else {
				verb = patch.PatchReplaceOp
			}
		}
		jp.Append(patch.NewJSONPatchOperation(verb, "/status/stateChangeRequests", changes))
	}
	if vm.Status.StartFailure != nil {
		jp.Append(patch.NewJSONPatchOperation(patch.PatchRemoveOp, "/status/startFailure", nil))
	}
	return jp.Bytes()
}

// BuildPatchSafeRestart creates a patch to restart a VM in case no other operations are present.
// This method respects other operations that was issued during VM reboot.
func BuildPatchSafeRestart(kvvm *virtv1.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) ([]byte, error) {
	// Restart only if current request is empty.
	if len(kvvm.Status.StateChangeRequests) > 0 {
		return nil, nil
	}
	restartRequest := []virtv1.VirtualMachineStateChangeRequest{
		{Action: virtv1.StopRequest, UID: &kvvmi.UID},
		{Action: virtv1.StartRequest},
	}
	jp := patch.NewJSONPatch()

	newStatus := virtv1.VirtualMachineStatus{}
	if equality.Semantic.DeepEqual(kvvm.Status, newStatus) {
		// Add /status if it's not exists.
		newStatus.StateChangeRequests = restartRequest
		jp.Append(patch.NewJSONPatchOperation(patch.PatchAddOp, "/status", newStatus))
	} else {
		// Set stateChangeRequests.
		jp.Append(patch.NewJSONPatchOperation(patch.PatchAddOp, "/status/stateChangeRequests", restartRequest))
	}
	return jp.Bytes()
}
