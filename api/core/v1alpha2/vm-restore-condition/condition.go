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

package vmrestorecondition

// Type represents the various condition types for the `VirtualMachineRestore`.
type Type string

const (
	// VirtualMachineSnapshotReadyToUseType indicates that the virtual machine snapshot has been successfully taken and is ready for restore.
	VirtualMachineSnapshotReadyToUseType Type = "VirtualMachineSnapshotReadyToUse"
	// VirtualMachineRestoreReadyType indicates that the virtual machine restore has been successfully completed.
	VirtualMachineRestoreReadyType Type = "VirtualMachineRestoreReady"
)

type (
	// VirtualMachineSnapshotReadyToUseReason represents the various reasons for the `VirtualMachineSnapshotReadyToUse` condition type.
	VirtualMachineSnapshotReadyToUseReason string
	// VirtualMachineRestoreReadyReason represents the various reasons for the `VirtualMachineRestoreReady` condition type.
	VirtualMachineRestoreReadyReason string
)

const (
	// VirtualMachineSnapshotNotFound indicates that the specified virtual machine snapshot is absent.
	VirtualMachineSnapshotNotFound VirtualMachineSnapshotReadyToUseReason = "VirtualMachineSnapshotNotFound"
	// VirtualMachineSnapshotNotReady indicates that the specified virtual machine snapshot is not ready, or resources, such as virtual images to be restored, are not present in the cluster.
	VirtualMachineSnapshotNotReady VirtualMachineSnapshotReadyToUseReason = "VirtualMachineSnapshotNotReady"
	// VirtualMachineSnapshotReadyToUse indicates that the specified virtual machine snapshot is ready to restore.
	VirtualMachineSnapshotReadyToUse VirtualMachineSnapshotReadyToUseReason = "VirtualMachineSnapshotReadyToUse"

	// VirtualMachineResourcesAreNotReady signifies that the virtual machine resources is not ready to the `force` restoration.
	VirtualMachineResourcesAreNotReady VirtualMachineRestoreReadyReason = "VirtualMachineResourcesAreNotReady"
	// VirtualMachineIsNotStopped signifies that the virtual machine is not ready to the `force` restoration.
	VirtualMachineIsNotStopped VirtualMachineRestoreReadyReason = "VirtualMachineIsNotStopped"
	// VirtualMachineSnapshotNotReadyToUse signifies that the virtual machine snapshot is not ready to use.
	VirtualMachineSnapshotNotReadyToUse VirtualMachineRestoreReadyReason = "VirtualMachineSnapshotNotReadyToUse"
	// VirtualMachineRestoreConflict signifies that the virtual machine cannot be restored as it's resources already exist.
	VirtualMachineRestoreConflict VirtualMachineRestoreReadyReason = "VirtualMachineRestoreConflict"
	// VirtualMachineRestoreFailed signifies that the restore process has failed.
	VirtualMachineRestoreFailed VirtualMachineRestoreReadyReason = "VirtualMachineRestoreFailed"
	// VirtualMachineRestoreReady signifies that the restore process is completed.
	VirtualMachineRestoreReady VirtualMachineRestoreReadyReason = "VirtualMachineRestoreReady"
)

func (t Type) String() string {
	return string(t)
}

func (r VirtualMachineSnapshotReadyToUseReason) String() string {
	return string(r)
}

func (r VirtualMachineRestoreReadyReason) String() string {
	return string(r)
}
