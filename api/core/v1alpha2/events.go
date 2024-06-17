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

package v1alpha2

const (
	// ReasonErrUnknownState is event reason that VMI has unexpected state
	ReasonErrUnknownState = "UnknownState"

	// ReasonErrWrongPVCSize is event reason that PVC has wrong size
	ReasonErrWrongPVCSize = "WrongPVCSize"

	// ReasonErrImportFailed is event reason that importer/uploader Pod is failed
	ReasonErrImportFailed = "ImportFailed"

	// ReasonErrGetProgressFailed is event reason about the failure of getting progress.
	ReasonErrGetProgressFailed = "GetProgressFailed"

	// ReasonClaimNotAvailable is event reason that VM cannot use defined claim.
	ReasonClaimNotAvailable = "ClaimNotAvailable"

	// ReasonClaimNotAssigned is event reason that claimed IP is not assigned in the guest VM.
	ReasonClaimNotAssigned = "ClaimNotAssigned"

	// ReasonCPUModelNotFound is event reason that defined cpu model not found.
	ReasonCPUModelNotFound = "CPUModelNotFound"

	// ReasonImportSucceeded is event reason that the import is successfully completed
	ReasonImportSucceeded = "ImportSucceeded"

	// ReasonImportSucceededToPVC is event reason that the import is successfully completed to PVC
	ReasonImportSucceededToPVC = "ImportSucceededToPVC"

	// ReasonHotplugPostponed is event reason that disk hotplug is not possible at the moment.
	ReasonHotplugPostponed = "HotplugPostponed"

	// ReasonVMWaitForBlockDevices is event reason that block devices used by VM are not ready yet.
	ReasonVMWaitForBlockDevices = "WaitForBlockDevices"

	// ReasonVMChangesApplied is event reason that changes applied from VM to underlying KVVM.
	ReasonVMChangesApplied = "ChangesApplied"

	// ReasonVMRestarted is event reason that VM restarted.
	ReasonVMRestarted = "VMRestarted"

	// ReasonVMLastAppliedSpecInvalid is event reason that JSON in last-applied-spec annotation is invalid.
	ReasonVMLastAppliedSpecInvalid = "VMLastAppliedSpecInvalid"

	// ReasonErrUploaderWaitDurationExpired is event reason that uploading time expired.
	ReasonErrUploaderWaitDurationExpired = "UploaderWaitDurationExpired"

	// ReasonErrVmNotSynced is event reason that vm is not synced.
	ReasonErrVmNotSynced = "VirtualMachineNotSynced "

	// ReasonErrVMOPNotPermitted is event reason that vmop is not permitted.
	ReasonErrVMOPNotPermitted = "VirtualMachineOperationNotPermitted"

	// ReasonErrVMOPFailed is event reason that operation is failed
	ReasonErrVMOPFailed = "VirtualMachineOperationFailed"

	// ReasonVMOPSucceeded is event reason that the operation is successfully completed
	ReasonVMOPSucceeded = "VirtualMachineOperationSucceeded"
)
