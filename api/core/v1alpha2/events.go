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

	// ReasonVMStarted is event reason that VM is about to start.
	ReasonVMStarted = "Started"

	// ReasonVMStopped is event reason that VM is about to stop.
	ReasonVMStopped = "Stopped"

	// ReasonVMRestarted is event reason that VM is about to restart.
	ReasonVMRestarted = "Restarted"

	// ReasonVMEvicted is event reason that VM is about to evict.
	ReasonVMEvicted = "Evicted"

	// ReasonVMMigrated is event reason that VM is about to migrate.
	ReasonVMMigrated = "Migrated"

	// ReasonVMLastAppliedSpecIsInvalid is event reason that JSON in last-applied-spec annotation is invalid.
	ReasonVMLastAppliedSpecIsInvalid = "LastAppliedSpecIsInvalid"

	// ReasonVMClassLastAppliedSpecInvalid is event reason that JSON in last-applied-spec annotation is invalid.
	ReasonVMClassLastAppliedSpecInvalid = "VMClassLastAppliedSpecInvalid"

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

	// ReasonVMOPStarted is event reason that the operation is started
	ReasonVMOPStarted = "VirtualMachineOperationStarted"

	// ReasonVMClassInUse is event reason that VMClass is used by virtual machine.
	ReasonVMClassInUse = "VirtualMachineClassInUse"

	// ReasonVDStorageClassWasDeleted is event reason that VDStorageClass was deleted.
	ReasonVDStorageClassWasDeleted = "VirtualDiskStorageClassWasDeleted"
	// ReasonVDStorageClassNotFound is event reason that VDStorageClass not found.
	ReasonVDStorageClassNotFound = "VirtualDiskStorageClassNotFound"
	// ReasonVDSpecChanged is event reason that VDStorageClass is chanded.
	ReasonVDSpecChanged = "VirtualDiskSpecChanged"
	// ReasonVDContainerRegistrySecretNotFound is event reason that VDContainerRegistrySecret not found.
	ReasonVDContainerRegistrySecretNotFound = "VirtualDiskContainerRegistrySecretNotFound"

	// ReasonVDResizingStarted is event reason that VD Resizing is started.
	ReasonVDResizingStarted = "VirtualDiskResizingStarted"
	// ReasonVDResizingCompleted is event reason that VD Resizing is completed.
	ReasonVDResizingCompleted = "VirtualDiskResizingCompleted"
	// ReasonVDResizingFailed is event reason that VD Resizing is failed.
	ReasonVDResizingFailed = "VirtualDiskResizingFailed"
	// ReasonVDResizingNotAvailable is event reason that VD Resizing is not available.
	ReasonVDResizingNotAvailable = "VirtualDiskResizingNotAvailable"

	// ReasonDataSourceSyncStarted is event reason that DataSource sync is started.
	ReasonDataSourceSyncStarted = "DataSourceImportStarted"
	// ReasonDataSourceSyncInProgress is event reason that DataSource sync is in progress.
	ReasonDataSourceSyncInProgress = "DataSourceImportInProgress"
	// ReasonDataSourceSyncCompleted is event reason that DataSource sync is completed.
	ReasonDataSourceSyncCompleted = "DataSourceImportCompleted"
	// ReasonDataSourceSyncFailed is event reason that DataSource sync is failed.
	ReasonDataSourceSyncFailed = "DataSourceImportFailed"
	// ReasonDataSourceQuotaExceeded is event reason that DataSource sync is failed because quota exceed.
	ReasonDataSourceQuotaExceeded = "DataSourceQuotaExceed"

	// ReasonDataSourceDiskProvisioningFailed is event reason that DataSource disk provisioning is failed.
	ReasonDataSourceDiskProvisioningFailed = "DataSourceImportDiskProvisioningFailed"

	// ReasonVMSnapshottingStarted is event reason that VirtualMachine snapshotting is started.
	ReasonVMSnapshottingStarted = "VirtualMachineSnapshottingStarted"

	// ReasonVMSnapshottingFrozen is event reason that the file system of VirtualMachine is frozen.
	ReasonVMSnapshottingFrozen = "VirtualMachineSnapshottingFrozen"

	// ReasonVMSnapshottingInProgress is event reason that VirtualMachine snapshotting is in progress.
	ReasonVMSnapshottingInProgress = "VirtualMachineSnapshottingInProgress"

	// ReasonVMSnapshottingThawed is event reason that the file system of VirtualMachine is thawed.
	ReasonVMSnapshottingThawed = "VirtualMachineSnapshottingThawed"

	// ReasonVMSnapshottingPending is event reason that VirtualMachine is not ready for snapshotting.
	ReasonVMSnapshottingPending = "VirtualMachineSnapshottingPending"

	// ReasonVMSnapshottingCompleted is event reason that VirtualMachine snapshotting is completed.
	ReasonVMSnapshottingCompleted = "VirtualMachineSnapshottingCompleted"

	// ReasonVMSnapshottingFailed is event reason that VirtualMachine snapshotting is failed.
	ReasonVMSnapshottingFailed = "VirtualMachineSnapshottingFailed"

	// ReasonVMIPAttached is event reason that VMIP is attached to VM
	ReasonVMIPAttached = "Attached"
	// ReasonVMIPNotAttached is event reason that VirtualMachineIPAddress is not attached to VirtualMachine.
	ReasonVMIPNotAttached = "NotAttached"

	// ReasonVMIPLeaseBound is the event reason indicating that a VirtualMachineIPLease is bound to a VirtualMachineIPAddress.
	ReasonVMIPLeaseBound = "Bound"
	// ReasonVMIPLeaseBoundFailed is the event reason indicating that the binding of a VirtualMachineIPLease to a VirtualMachineIPAddress has failed.
	ReasonVMIPLeaseBoundFailed = "Failed"
)
