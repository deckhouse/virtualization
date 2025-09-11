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

	// TypeSignalSent is a type for condition that indicates operation signal has been sent.
	TypeSignalSent Type = "SignalSent"

	// TypeRestoreCompleted is a type for condition that indicates success of restore.
	TypeRestoreCompleted Type = "RestoreCompleted"

	// TypeCloneCompleted is a type for condition that indicates success of clone.
	TypeCloneCompleted Type = "CloneCompleted"

	// TypeMaintenanceMode is a type for condition that indicates VMOP has put VM in maintenance mode.
	TypeMaintenanceMode Type = "MaintenanceMode"

	// TypeSnapshotReady is a type for condition that indicates snapshot is ready for clone operation.
	TypeSnapshotReady Type = "SnapshotReady"
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

	// ReasonNotReadyToBeExecuted is a ReasonCompleted indicating that the operation is not ready to be executed.
	ReasonNotReadyToBeExecuted ReasonCompleted = "NotReadyToBeExecuted"

	// ReasonRestartInProgress is a ReasonCompleted indicating that the restart signal has been sent and restart is in progress.
	ReasonRestartInProgress ReasonCompleted = "RestartInProgress"

	// ReasonStartInProgress is a ReasonCompleted indicating that the start signal has been sent and start is in progress.
	ReasonStartInProgress ReasonCompleted = "StartInProgress"

	// ReasonStopInProgress is a ReasonCompleted indicating that the stop signal has been sent and stop is in progress.
	ReasonStopInProgress ReasonCompleted = "StopInProgress"

	// ReasonRestoreInProgress is a ReasonCompleted indicating that the restore operation is in progress.
	ReasonRestoreInProgress ReasonCompleted = "RestoreInProgress"

	// ReasonCloneInProgress is a ReasonCompleted indicating that the clone operation is in progress.
	ReasonCloneInProgress ReasonCompleted = "CloneInProgress"

	// ReasonMigrationPending is a ReasonCompleted indicating that the migration process has been initiated but not yet started.
	ReasonMigrationPending ReasonCompleted = "MigrationPending"

	// ReasonMigrationPrepareTarget is a ReasonCompleted indicating that the target environment is being prepared for migration.
	ReasonMigrationPrepareTarget ReasonCompleted = "MigrationPrepareTarget"

	// ReasonMigrationTargetReady is a ReasonCompleted indicating that the target environment is ready to accept the migration.
	ReasonMigrationTargetReady ReasonCompleted = "MigrationTargetReady"

	// ReasonMigrationRunning is a ReasonCompleted indicating that the migration process is currently in progress.
	ReasonMigrationRunning ReasonCompleted = "MigrationRunning"

	// ReasonOtherMigrationInProgress is a ReasonCompleted indicating that there are other migrations in progress.
	ReasonOtherMigrationInProgress ReasonCompleted = "OtherMigrationInProgress"

	// ReasonQuotaExceeded is a completed reason that indicates the project's quota has been exceeded and the migration has been paused.
	ReasonQuotaExceeded ReasonCompleted = "QuotaExceeded"

	// ReasonOperationFailed is a ReasonCompleted indicating that operation has failed.
	ReasonOperationFailed ReasonCompleted = "OperationFailed"

	// ReasonOperationCompleted is a ReasonCompleted indicating that operation is completed.
	ReasonOperationCompleted ReasonCompleted = "OperationCompleted"
)

// ReasonRestoreCompleted represents specific reasons for the 'RestoreCompleted' condition type.
type ReasonRestoreCompleted string

func (r ReasonRestoreCompleted) String() string {
	return string(r)
}

const (
	// ReasonRestoreInProgress is a ReasonRestoreCompleted indicating that the restore operation is in progress.
	ReasonRestoreOperationInProgress ReasonRestoreCompleted = "RestoreInProgress"

	// ReasonRestoreOperationCompleted is a ReasonRestoreCompleted indicating that the restore operation has completed successfully.
	ReasonRestoreOperationCompleted ReasonRestoreCompleted = "RestoreCompleted"

	// ReasonDryRunOperationCompleted is a ReasonRestoreCompleted indicating that the restore dry run operation has completed successfully.
	ReasonDryRunOperationCompleted ReasonRestoreCompleted = "RestoreDryRunCompleted"

	// ReasonRestoreOperationFailed is a ReasonRestoreCompleted indicating that operation has failed.
	ReasonRestoreOperationFailed ReasonRestoreCompleted = "RestoreFailed"
)

// ReasonCloneCompleted represents specific reasons for the 'CloneCompleted' condition type.
type ReasonCloneCompleted string

func (r ReasonCloneCompleted) String() string {
	return string(r)
}

const (
	// ReasonCloneOperationInProgress is a ReasonCloneCompleted indicating that the clone operation is in progress.
	ReasonCloneOperationInProgress ReasonCloneCompleted = "CloneInProgress"

	// ReasonCloneOperationCompleted is a ReasonCloneCompleted indicating that the clone operation has completed successfully.
	ReasonCloneOperationCompleted ReasonCloneCompleted = "CloneCompleted"

	// ReasonCloneOperationFailed is a ReasonCloneCompleted indicating that clone operation has failed.
	ReasonCloneOperationFailed ReasonCloneCompleted = "CloneFailed"
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

// ReasonMaintenanceMode represents specific reasons for the 'MaintenanceMode' condition type.
type ReasonMaintenanceMode string

func (r ReasonMaintenanceMode) String() string {
	return string(r)
}

const (
	// ReasonMaintenanceModeEnabled is a ReasonMaintenanceMode indicating that VM is in maintenance mode for restore operation.
	ReasonMaintenanceModeEnabled ReasonMaintenanceMode = "MaintenanceModeEnabled"

	// ReasonMaintenanceModeDisabled is a ReasonMaintenanceMode indicating that VM has exited maintenance mode.
	ReasonMaintenanceModeDisabled ReasonMaintenanceMode = "MaintenanceModeDisabled"

	// ReasonMaintenanceModeFailure is a ReasonMaintenanceMode indicating that maintenance mode operation failed.
	ReasonMaintenanceModeFailure ReasonMaintenanceMode = "MaintenanceModeFailure"
)

// ReasonSnapshotReady represents specific reasons for the 'SnapshotReady' condition type.
type ReasonSnapshotReady string

func (r ReasonSnapshotReady) String() string {
	return string(r)
}

const (
	// ReasonSnapshotInProgress is a ReasonSnapshotReady indicating that snapshot creation is in progress.
	ReasonSnapshotInProgress ReasonSnapshotReady = "SnapshotInProgress"

	// ReasonSnapshotOperationReady is a ReasonSnapshotReady indicating that snapshot is ready for clone operation.
	ReasonSnapshotOperationReady ReasonSnapshotReady = "SnapshotReady"

	// ReasonSnapshotCleanedUp is a ReasonSnapshotReady indicating that snapshot has been cleaned up.
	ReasonSnapshotCleanedUp ReasonSnapshotReady = "SnapshotCleanedUp"

	// ReasonSnapshotFailed is a ReasonSnapshotReady indicating that snapshot operation failed.
	ReasonSnapshotFailed ReasonSnapshotReady = "SnapshotFailed"
)
