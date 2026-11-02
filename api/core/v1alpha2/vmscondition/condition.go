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

package vmscondition

// Type represents the various condition types for the `VirtualMachineSnapshot`.
type Type string

const (
	// VirtualMachineReadyType indicates that the `VirtualMachine` is ready for snapshotting.
	VirtualMachineReadyType Type = "VirtualMachineReady"
	// VirtualMachineSnapshotReadyType indicates that the virtual machine snapshot has been successfully taken and is ready for restore.
	VirtualMachineSnapshotReadyType Type = "VirtualMachineSnapshotReady"
)

type (
	// VirtualMachineReadyReason represents the various reasons for the `VirtualMachineReady` condition type.
	VirtualMachineReadyReason string
	// VirtualMachineSnapshotReadyReason represents the various reasons for the `VirtualMachineSnapshotReady` condition type.
	VirtualMachineSnapshotReadyReason string
)

const (
	// VirtualMachineReady signifies that the virtual machine is ready for snapshotting, allowing the snapshot process to begin.
	VirtualMachineReady VirtualMachineReadyReason = "VirtualMachineReady"
	// VirtualMachineNotReadyForSnapshotting signifies that the virtual machine is not ready for snapshotting, preventing the snapshot process from starting.
	VirtualMachineNotReadyForSnapshotting VirtualMachineReadyReason = "VirtualMachineNotReadyForSnapshotting"

	// WaitingForTheVirtualMachine signifies that the snapshot process is waiting for the virtual machine to become ready for snapshotting.
	WaitingForTheVirtualMachine VirtualMachineSnapshotReadyReason = "WaitingForTheVirtualMachine"
	// BlockDevicesNotReady signifies that the snapshotting process cannot begin because the block devices of the virtual machine are not ready.
	BlockDevicesNotReady VirtualMachineSnapshotReadyReason = "BlockDevicesNotReady"
	// PotentiallyInconsistent signifies that the snapshotting process cannot begin because creating a snapshot of the running virtual machine might result in an inconsistent snapshot.
	PotentiallyInconsistent VirtualMachineSnapshotReadyReason = "PotentiallyInconsistent"
	// VirtualDiskSnapshotLost signifies that the underlying `VirtualDiskSnapshot` is lost: cannot restore the virtual machine using this snapshot.
	VirtualDiskSnapshotLost VirtualMachineSnapshotReadyReason = "VirtualDiskSnapshotLost"
	// FileSystemFreezing signifies that the `VirtualMachineSnapshot` resource is in the process of freezing the filesystem of the virtual machine.
	FileSystemFreezing VirtualMachineSnapshotReadyReason = "FileSystemFreezing"
	// FileSystemUnfreezing signifies that the `VirtualMachineSnapshot` resource is in the process of unfreezing the filesystem of the virtual machine.
	FileSystemUnfreezing VirtualMachineSnapshotReadyReason = "FileSystemUnfreezing"
	// Snapshotting signifies that the `VirtualMachineSnapshot` resource is in the process of taking a snapshot of the virtual machine.
	Snapshotting VirtualMachineSnapshotReadyReason = "Snapshotting"
	// VirtualMachineSnapshotReady signifies that the snapshot process is complete and the `VirtualMachineSnapshot` is ready for use.
	VirtualMachineSnapshotReady VirtualMachineSnapshotReadyReason = "VirtualMachineSnapshotReady"
	// VirtualMachineSnapshotFailed signifies that the snapshot process has failed.
	VirtualMachineSnapshotFailed VirtualMachineSnapshotReadyReason = "VirtualMachineSnapshotFailed"
	// VirtualMachineBusy signifies that the snapshot has failed because another snapshot holds the virtual machine.
	VirtualMachineBusy VirtualMachineSnapshotReadyReason = "VirtualMachineBusy"
	// FilesystemAlreadyFrozen signifies that the snapshot has failed because the filesystem of the virtual machine was already frozen when the snapshot started.
	FilesystemAlreadyFrozen VirtualMachineSnapshotReadyReason = "FilesystemAlreadyFrozen"
	// VirtualMachineNotReady signifies that the snapshot has failed because the virtual machine was not ready to be snapshotted: it was just created, or its internal state was removed.
	VirtualMachineNotReady VirtualMachineSnapshotReadyReason = "VirtualMachineNotReady"
	// FilesystemFreezeFailed signifies that the snapshot has failed because a consistent snapshot was required and the filesystem of the virtual machine could not be frozen.
	FilesystemFreezeFailed VirtualMachineSnapshotReadyReason = "FilesystemFreezeFailed"
	// VirtualMachineStateChanged signifies that the snapshot has failed because the virtual machine stopped, started, restarted or started migrating while the snapshot was being taken.
	VirtualMachineStateChanged VirtualMachineSnapshotReadyReason = "VirtualMachineStateChanged"
	// FilesystemUnfrozenUnexpectedly signifies that the snapshot has failed because the filesystem of the virtual machine was unfrozen while the snapshot was being taken.
	FilesystemUnfrozenUnexpectedly VirtualMachineSnapshotReadyReason = "FilesystemUnfrozenUnexpectedly"
	// DeadlineExceeded signifies that the snapshot has failed because it was not taken before the deadline the virtual machine was reserved for it.
	DeadlineExceeded VirtualMachineSnapshotReadyReason = "DeadlineExceeded"
	// AbortedByModuleUpdate signifies that the snapshot has failed because it was started before an update of the virtualization module, and the updated module cannot complete it.
	AbortedByModuleUpdate VirtualMachineSnapshotReadyReason = "AbortedByModuleUpdate"
)

func (t Type) String() string {
	return string(t)
}

func (r VirtualMachineReadyReason) String() string {
	return string(r)
}

func (r VirtualMachineSnapshotReadyReason) String() string {
	return string(r)
}
