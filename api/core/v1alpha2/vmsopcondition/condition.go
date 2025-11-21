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

package vmsopcondition

type Type string

func (t Type) String() string {
	return string(t)
}

const (
	// TypeCompleted is a type for condition that indicates operation is complete.
	TypeCompleted Type = "Completed"

	// TypeCreateVirtualMachineCompleted is a type for condition that indicates success of clone.
	TypeCreateVirtualMachineCompleted Type = "CreateVirtualMachineCompleted"
)

// ReasonCompleted represents specific reasons for the 'Completed' condition type.
type ReasonCompleted string

func (r ReasonCompleted) String() string {
	return string(r)
}

const (
	// ReasonVirtualMachineSnapshotNotFound is a ReasonCompleted indicating that the specified virtual machine is absent.
	ReasonVirtualMachineSnapshotNotFound ReasonCompleted = "VirtualMachineSnapshotNotFound"

	// ReasonNotApplicableForVMSPhase is a ReasonCompleted indicating that the specified operation type is not applicable for the virtual machine phase.
	ReasonNotApplicableForVMSPhase ReasonCompleted = "NotApplicableForVirtualMachinePhase"

	// ReasonNotReadyToBeExecuted is a ReasonCompleted indicating that the operation is not ready to be executed.
	ReasonNotReadyToBeExecuted ReasonCompleted = "NotReadyToBeExecuted"

	// ReasonCreateVirtualMachineInProgress is a ReasonCompleted indicating that the clone operation is in progress.
	ReasonCreateVirtualMachineInProgress ReasonCompleted = "CreateVirtualMachineInProgress"

	// ReasonOperationFailed is a ReasonCompleted indicating that operation has failed.
	ReasonOperationFailed ReasonCompleted = "OperationFailed"

	// ReasonOperationCompleted is a ReasonCompleted indicating that operation is completed.
	ReasonOperationCompleted ReasonCompleted = "OperationCompleted"
)

// ReasonCreateVirtualMachineCompleted represents specific reasons for the 'CreateVirtualMachineCompleted' condition type.
type ReasonCreateVirtualMachineCompleted string

func (r ReasonCreateVirtualMachineCompleted) String() string {
	return string(r)
}

const (
	// ReasonCreateVirtualMachineOperationInProgress is a ReasonCreateVirtualMachineCompleted indicating that the clone operation is in progress.
	ReasonCreateVirtualMachineOperationInProgress ReasonCreateVirtualMachineCompleted = "CreateVirtualMachineInProgress"

	// ReasonCreateVirtualMachineOperationCompleted is a ReasonCreateVirtualMachineCompleted indicating that the clone operation has completed successfully.
	ReasonCreateVirtualMachineOperationCompleted ReasonCreateVirtualMachineCompleted = "CreateVirtualMachineCompleted"

	// ReasonCreateVirtualMachineOperationFailed is a ReasonCreateVirtualMachineCompleted indicating that clone operation has failed.
	ReasonCreateVirtualMachineOperationFailed ReasonCreateVirtualMachineCompleted = "CreateVirtualMachineFailed"
)
