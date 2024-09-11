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

// Reason represents specific reasons for the 'Completed' condition type.
type Reason string

func (r Reason) String() string {
	return string(r)
}

const (
	ReasonUnknown Reason = "Unknown"

	// ReasonSignalSentError is a Reason indicating an error occurred while sending powerstate signal to the VM.
	ReasonSignalSentError Reason = "ReasonSignalSentError"

	// ReasonSignalSentSuccess is a Reason indicating that signal is sent to the VM.
	ReasonSignalSentSuccess Reason = "ReasonSignalSentSuccess"

	// ReasonVirtualMachineNotFound is a Reason indicating that the specified virtual machine is absent.
	ReasonVirtualMachineNotFound Reason = "ReasonVirtualMachineNotFound"

	// ReasonNotApplicableForRunPolicy is a Reason indicating that the specified operation type is not appilicable for the virtual machine runPolicy.
	ReasonNotApplicableForRunPolicy Reason = "ReasonNotApplicableForRunPolicy"

	// ReasonNotApplicableForVMPhase is a Reason indicating that the specified operation type is not appilicable for the virtual machine phase.
	ReasonNotApplicableForVMPhase Reason = "ReasonNotApplicableForVMPhase"

	// ReasonOtherOperationsAreInProgress is a Reason indicating that there are other operations in progress.
	ReasonOtherOperationsAreInProgress Reason = "ReasonOtherOperationsAreInProgress"

	// ReasonRestartInProgress is a Reason indicating that the restart signal has been sent and restart is in progress.
	ReasonRestartInProgress Reason = "ReasonRestartInProgress"

	// ReasonStartInProgress is a Reason indicating that the start signal has been sent and start is in progress.
	ReasonStartInProgress Reason = "ReasonStartInProgress"

	// ReasonStopInProgress is a Reason indicating that the stop signal has been sent and stop is in progress.
	ReasonStopInProgress Reason = "ReasonStopInProgress"

	// ReasonOperationFailed is a Reason indicating that operation has failed.
	ReasonOperationFailed Reason = "ReasonOperationFailed"

	// ReasonOperationCompleted is a Reason indicating that operation is completed.
	ReasonOperationCompleted Reason = "ReasonOperationCompleted"
)
