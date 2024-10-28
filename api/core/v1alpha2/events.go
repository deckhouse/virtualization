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
	// ReasonVDAlreadyInUse is event reason that VirtualDisk was not attached to VirtualMachine, because VirtualDisk attached to another VirtualMachine.
	ReasonVDAlreadyInUse = "VirtualDiskAlreadyInUse"
	// ReasonVMChangesApplied is event reason that changes applied from VM to underlying KVVM.
	ReasonVMChangesApplied = "ChangesApplied"

	// ReasonVMRestarted is event reason that VM restarted.
	ReasonVMRestarted = "VMRestarted"

	// ReasonVMLastAppliedSpecInvalid is event reason that JSON in last-applied-spec annotation is invalid.
	ReasonVMLastAppliedSpecInvalid = "VMLastAppliedSpecInvalid"

	// ReasonErrVmNotSynced is event reason that vm is not synced.
	ReasonErrVmNotSynced = "VirtualMachineNotSynced"

	// ReasonErrVmSynced is event reason that vm is synced.
	ReasonErrVmSynced = "VirtualMachineSynced"

	// ReasonErrRestartAwaitingChanges is event reason indicating that the vm has pending changes requiring a restart.
	ReasonErrRestartAwaitingChanges = "RestartAwaitingChanges"

	// ReasonErrVMOPFailed is event reason that operation is failed
	ReasonErrVMOPFailed = "VirtualMachineOperationFailed"

	// ReasonVMOPSucceeded is event reason that the operation is successfully completed
	ReasonVMOPSucceeded = "VirtualMachineOperationSucceeded"

	// ReasonVMClassInUse is event reason that VMClass is used by virtual machine.
	ReasonVMClassInUse = "VirtualMachineClassInUse"
)
