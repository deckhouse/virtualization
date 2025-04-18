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

package vmopcondition

type Type string

func (t Type) String() string {
	return string(t)
}

const (
	// TypeCompleted is a type for condition that indicates operation is complete.
	TypeCompleted Type = "Completed"

	// SignalSentType is a type for condition that indicates operation signal has been sent.
	SignalSentType Type = "SignalSent"
)

// ReasonCompleted represents specific reasons for the 'Completed' condition type.
type ReasonCompleted string

func (r ReasonCompleted) String() string {
	return string(r)
}

const (
	// ReasonVirtualMachineNotFound is a ReasonCompleted indicating that the specified virtual machine is absent.
	ReasonVirtualMachineNotFound ReasonCompleted = "VirtualMachineNotFound"

	// ReasonNotApplicableForRunPolicy is a ReasonCompleted indicating that the specified operation type is not applicable for the virtual machine runPolicy.
	ReasonNotApplicableForRunPolicy ReasonCompleted = "NotApplicableForVirtualMachineRunPolicy"

	// ReasonNotApplicableForVMPhase is a ReasonCompleted indicating that the specified operation type is not applicable for the virtual machine phase.
	ReasonNotApplicableForVMPhase ReasonCompleted = "NotApplicableForVirtualMachinePhase"

	// ReasonNotApplicableForLiveMigrationPolicy is a ReasonCompleted indicating that the specified operation type is not applicable for the virtual machine live migration policy.
	ReasonNotApplicableForLiveMigrationPolicy ReasonCompleted = "NotApplicableForLiveMigrationPolicy"

	// ReasonOtherOperationsAreInProgress is a ReasonCompleted indicating that there are other operations in progress.
	ReasonOtherOperationsAreInProgress ReasonCompleted = "OtherOperationsAreInProgress"

	// ReasonRestartInProgress is a ReasonCompleted indicating that the restart signal has been sent and restart is in progress.
	ReasonRestartInProgress ReasonCompleted = "RestartInProgress"

	// ReasonStartInProgress is a ReasonCompleted indicating that the start signal has been sent and start is in progress.
	ReasonStartInProgress ReasonCompleted = "StartInProgress"

	// ReasonStopInProgress is a ReasonCompleted indicating that the stop signal has been sent and stop is in progress.
	ReasonStopInProgress ReasonCompleted = "StopInProgress"

	// ReasonMigrationInProgress is a ReasonCompleted indicating that the migrate signal has been sent and stop is in progress.
	ReasonMigrationInProgress ReasonCompleted = "MigrationInProgress"

	// ReasonOtherMigrationInProgress is a ReasonCompleted indicating that there are other migrations in progress.
	ReasonOtherMigrationInProgress ReasonCompleted = "OtherMigrationInProgress"

	// ReasonOperationFailed is a ReasonCompleted indicating that operation has failed.
	ReasonOperationFailed ReasonCompleted = "OperationFailed"

	// ReasonOperationCompleted is a ReasonCompleted indicating that operation is completed.
	ReasonOperationCompleted ReasonCompleted = "OperationCompleted"
)

// ReasonCompleted represents specific reasons for the 'SignalSent' condition type.
type ReasonSignalSent string

func (r ReasonSignalSent) String() string {
	return string(r)
}

const (
	// ReasonSignalSentError is a ReasonCompleted indicating an error occurred while sending powerstate signal to the VM.
	ReasonSignalSentError ReasonSignalSent = "SignalSentError"

	// ReasonSignalSentSuccess is a ReasonCompleted indicating that signal is sent to the VM.
	ReasonSignalSentSuccess ReasonSignalSent = "SignalSentSuccess"
)
