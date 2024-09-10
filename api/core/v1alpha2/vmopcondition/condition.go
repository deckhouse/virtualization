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

type Type = string

const (
	// CompletedType is a type for condition that indicates operation is complete.
	CompletedType Type = "Completed"

	// SignalSentType is a type for condition that indicates operation signal has been sent.
	SignalSentType Type = "SignalHasBeenSent"
)

type (
	// CompletedReason represents specific reasons for the 'Completed' condition type.
	CompletedReason = string
)

const (
	// VirtualMachineNotFound is a CompletedReason indicating that the specified virtual machine is absent.
	VirtualMachineNotFound CompletedReason = "VirtualMachineNotFound"

	// NotApplicableForRunPolicy is a CompletedReason indicating that the specified operation type is not appilicable for the virtual machine runPolicy.
	NotApplicableForRunPolicy CompletedReason = "NotApplicableForRunPolicy"

	// NotApplicableForVMPhase is a CompletedReason indicating that the specified operation type is not appilicable for the virtual machine phase.
	NotApplicableForVMPhase CompletedReason = "NotApplicableForVMPhase"

	// OtherOperationsAreInProgress is a CompletedReason indicating that there are other operations in progress.
	OtherOperationsAreInProgress CompletedReason = "OtherOperationsAreInProgress"

	// // WaitForOtherOperations is a CompletedReason indicating that there are other operations in progress.
	// WaitForOtherOperations CompletedReason = "WaitForOtherOperations"

	// RestartInProgress is a CompletedReason indicating that the restart signal has been sent and restart is in progress.
	RestartInProgress CompletedReason = "RestartInProgress"

	// StartInProgress is a CompletedReason indicating that the start signal has been sent and start is in progress.
	StartInProgress CompletedReason = "StartInProgress"

	// StopInProgress is a CompletedReason indicating that the stop signal has been sent and stop is in progress.
	StopInProgress CompletedReason = "StopInProgress"

	// OperationFailed is a CompletedReason indicating that operation has failed.
	OperationFailed CompletedReason = "OperationFailed"
)
