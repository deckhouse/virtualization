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

	// ReasonVMStartFailed is an event reason indicating that the start of the VM failed.
	ReasonVMStartFailed = "Failed"

	// ReasonVMStopFailed is an event reason indicating that the stop of the VM failed.
	ReasonVMStopFailed = "Failed"

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

	// ReasonErrVMOPPending is event reason that operation is pending
	ReasonErrVMOPPending = "VirtualMachineOperationPending"

	// ReasonVMOPSucceeded is event reason that the operation is successfully completed
	ReasonVMOPSucceeded = "VirtualMachineOperationSucceeded"

	// ReasonVMOPStarted is event reason that the operation is started
	ReasonVMOPStarted = "VirtualMachineOperationStarted"

	// ReasonVMOPInProgress is event reason that the operation is in progress
	ReasonVMOPInProgress = "VirtualMachineOperationInProgress"

	// ReasonVMSOPStarted is event reason that the operation is started
	ReasonVMSOPStarted = "VirtualMachineSnaphotOperationStarted"

	// ReasonErrVMSOPFailed is event reason that operation is failed
	ReasonErrVMSOPFailed = "VirtualMachineSnapshotOperationFailed"

	// ReasonVMSOPSucceeded is event reason that the operation is successfully completed
	ReasonVMSOPSucceeded = "VirtualMachineSnapshotOperationSucceeded"

	// ReasonVMSOPInProgress is event reason that the operation is in progress
	ReasonVMSOPInProgress = "VirtualMachineSnapshotOperationInProgress"

	// ReasonVDSpecHasBeenChanged is event reason that spec of virtual disk has been changed.
	ReasonVDSpecHasBeenChanged = "VirtualDiskSpecHasBeenChanged"
	// ReasonVISpecHasBeenChanged is event reason that spec of virtual image has been changed.
	ReasonVISpecHasBeenChanged = "VirtualImageSpecHasBeenChanged"

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

	// ReasonVIStorageClassNotFound is event reason that VIStorageClass not found.
	ReasonVIStorageClassNotFound = "VirtualImageStorageClassNotFound"

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

	// ReasonImageOperationPostponedDueToDVCRGarbageCollection is event reason that operation is postponed until the end of DVCR garbage collection.
	ReasonImageOperationPostponedDueToDVCRGarbageCollection = "ImageOperationPostponedDueToDVCRGarbageCollection"
	// ReasonImageOperationContinueAfterDVCRGarbageCollection is event reason that operation is resumed after DVCR garbage collection is finished.
	ReasonImageOperationContinueAfterDVCRGarbageCollection = "ImageOperationContinueAfterDVCRGarbageCollection"

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

	// ReasonAttached is event reason that VirtualMachineIPAddress is attached to VM.
	ReasonAttached = "Attached"
	// ReasonNotAttached is event reason that VirtualMachineIPAddress is not attached to VirtualMachine.
	ReasonNotAttached = "NotAttached"

	// ReasonIPAddressHasBeenAllocated is the event reason indicating that a new IP address has been allocated.
	ReasonIPAddressHasBeenAllocated = "IPAddressHasBeenAllocated"
	// ReasonBound is the event reason indicating that a VirtualMachineIPLease is bound to a VirtualMachineIPAddress.
	ReasonBound = "Bound"
	// ReasonReleased is the event reason indicating that a VirtualMachineIPLease is released.
	ReasonReleased = "Released"
	// ReasonFailed is the event reason indicating that the binding of a VirtualMachineIPLease to a VirtualMachineIPAddress has failed.
	ReasonFailed = "Failed"

	// ReasonVMClassSizingPoliciesWereChanged is event reason indicating that VMClass sizing policies were changed.
	ReasonVMClassSizingPoliciesWereChanged = "SizingPoliciesWereChanged"

	// ReasonVMClassAvailableNodesListEmpty is event reason indicating that VMClass has no available nodes.
	ReasonVMClassAvailableNodesListEmpty = "AvailableNodesListEmpty"

	// ReasonVMClassNodesWereUpdated is event reason indicating that VMClass available nodes list was updated.
	ReasonVMClassNodesWereUpdated = "NodesWereUpdated"

	// ReasonVolumeMigrationCannotBeProcessed is event reason indicating that volume migration cannot be processed.
	ReasonVolumeMigrationCannotBeProcessed = "VolumeMigrationCannotBeProcessed"
)
