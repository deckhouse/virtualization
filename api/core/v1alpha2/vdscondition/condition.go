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

package vdscondition

// Type represents the various condition types for the `VirtualDiskSnapshot`.
type Type string

func (s Type) String() string {
	return string(s)
}

const (
	// VirtualDiskReadyType indicates that the source `VirtualDisk` is ready for snapshotting.
	VirtualDiskReadyType Type = "VirtualDiskReady"
	// VirtualDiskSnapshotReadyType indicates that the virtual disk snapshot has been successfully taken and is ready for use.
	VirtualDiskSnapshotReadyType Type = "VirtualDiskSnapshotReady"
)

type (
	// VirtualDiskReadyReason represents the various reasons for the `VirtualDiskReady` condition type.
	VirtualDiskReadyReason string
	// VirtualDiskSnapshotReadyReason represents the various reasons for the `VirtualDiskSnapshotReady` condition type.
	VirtualDiskSnapshotReadyReason string
)

func (s VirtualDiskReadyReason) String() string {
	return string(s)
}

func (s VirtualDiskSnapshotReadyReason) String() string {
	return string(s)
}

func (s Type) VirtualDiskSnapshotReadyReason() string {
	return string(s)
}

const (
	// VirtualDiskReady signifies that the source virtual disk is ready for snapshotting, allowing the snapshot process to begin.
	VirtualDiskReady VirtualDiskReadyReason = "VirtualDiskReady"
	// VirtualDiskNotReadyForSnapshotting signifies that the source virtual disk is not ready for snapshotting, preventing the snapshot process from starting.
	VirtualDiskNotReadyForSnapshotting VirtualDiskReadyReason = "VirtualDiskNotReadyForSnapshotting"

	// WaitingForTheVirtualDisk signifies that the snapshot process is waiting for the virtual disk to become ready for snapshotting.
	WaitingForTheVirtualDisk VirtualDiskSnapshotReadyReason = "WaitingForTheVirtualDisk"
	// PotentiallyInconsistent signifies that the snapshotting process cannot begin because creating a snapshot of virtual disk attached to the running virtual machine might result in an inconsistent snapshot.
	PotentiallyInconsistent VirtualDiskSnapshotReadyReason = "PotentiallyInconsistent"
	// VolumeSnapshotLost signifies that the underlying `VolumeSnapshot` is lost: cannot use the virtual disk snapshot as a data source.
	VolumeSnapshotLost VirtualDiskSnapshotReadyReason = "Lost"
	// FileSystemFreezing signifies that the `VirtualDiskSnapshot` resource is in the process of freezing the filesystem of the virtual machine associated with the source virtual disk.
	FileSystemFreezing VirtualDiskSnapshotReadyReason = "FileSystemFreezing"
	// FileSystemUnfreezing signifies that the `VirtualDiskSnapshot` resource is in the process of unfreezing the filesystem of the virtual machine associated with the source virtual disk.
	FileSystemUnfreezing VirtualDiskSnapshotReadyReason = "FileSystemUnfreezing"
	// Snapshotting signifies that the `VirtualDiskSnapshot` resource is in the process of taking a snapshot of the virtual disk.
	Snapshotting VirtualDiskSnapshotReadyReason = "Snapshotting"
	// VirtualDiskSnapshotReady signifies that the snapshot process is complete and the `VirtualDiskSnapshot` is ready for use.
	VirtualDiskSnapshotReady VirtualDiskSnapshotReadyReason = "VirtualDiskSnapshotReady"
	// VirtualDiskSnapshotFailed signifies that the snapshot process has failed.
	VirtualDiskSnapshotFailed VirtualDiskSnapshotReadyReason = "VirtualDiskSnapshotFailed"
)
