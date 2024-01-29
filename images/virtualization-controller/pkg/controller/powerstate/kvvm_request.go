package powerstate

import (
	"fmt"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"k8s.io/apimachinery/pkg/api/equality"
	kvv1 "kubevirt.io/api/core/v1"
)

// BuildPatch creates a patch to request VM state changing via updating KVVM status.
//
// Some combinations lead to an error to not interfere with kvvm controller:
//
// current  /  desired         stop      start     restart(stop+start)
// stop                        replace   error     error
// start                       replace   error     error
// restart(stop+start)         replace   error     error
// empty                       add       add       add
func BuildPatch(vm *kvv1.VirtualMachine, changes ...kvv1.VirtualMachineStateChangeRequest) ([]byte, error) {
	jp := patch.NewJsonPatch()
	// Special case: if there's no status field at all, add one.
	newStatus := kvv1.VirtualMachineStatus{}
	if equality.Semantic.DeepEqual(vm.Status, newStatus) {
		newStatus.StateChangeRequests = changes
		jp.Append(patch.NewJsonPatchOperation(patch.PatchAddOp, "/status", newStatus))
	} else {
		verb := patch.PatchAddOp
		failOnConflict := true
		if len(changes) == 1 && changes[0].Action == kvv1.StopRequest {
			// If this is a stopRequest, replace all existing StateChangeRequests.
			failOnConflict = false
		}
		if len(vm.Status.StateChangeRequests) != 0 {
			if failOnConflict {
				return nil, fmt.Errorf("unable to complete request: stop/start already underway")
			} else {
				verb = patch.PatchReplaceOp
			}
		}
		jp.Append(patch.NewJsonPatchOperation(verb, "/status/stateChangeRequests", changes))
	}
	if vm.Status.StartFailure != nil {
		jp.Append(patch.NewJsonPatchOperation(patch.PatchRemoveOp, "/status/startFailure", nil))
	}
	return jp.Bytes()
}

// BuildPatchSafeRestart creates a patch to restart a VM in case no other operations are present.
// This method respects other operations that was issued during VM reboot.
func BuildPatchSafeRestart(kvvm *kvv1.VirtualMachine, kvvmi *kvv1.VirtualMachineInstance) ([]byte, error) {
	// Restart only if current request is empty.
	if len(kvvm.Status.StateChangeRequests) > 0 {
		return nil, nil
	}
	restartRequest := []kvv1.VirtualMachineStateChangeRequest{
		{Action: kvv1.StopRequest, UID: &kvvmi.UID},
		{Action: kvv1.StartRequest},
	}
	jp := patch.NewJsonPatch()

	newStatus := kvv1.VirtualMachineStatus{}
	if equality.Semantic.DeepEqual(kvvm.Status, newStatus) {
		// Add /status if it's not exists.
		newStatus.StateChangeRequests = restartRequest
		jp.Append(patch.NewJsonPatchOperation(patch.PatchAddOp, "/status", newStatus))
	} else {
		// Set stateChangeRequests.
		jp.Append(patch.NewJsonPatchOperation(patch.PatchAddOp, "/status/stateChangeRequests", restartRequest))
	}
	return jp.Bytes()
}
