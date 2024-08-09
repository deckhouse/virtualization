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

package vmbdacondition

// Type represents the various condition types for the `VirtualMachineBlockDeviceAttachment`.
type Type = string

const (
	// BlockDeviceReadyType indicates that the block device (for example, a `VirtualDisk`) is ready to be hot-plugged.
	BlockDeviceReadyType Type = "BlockDeviceReady"
	// VirtualMachineReadyType indicates that the virtual machine is ready for hot-plugging a block device.
	VirtualMachineReadyType Type = "VirtualMachineReady"
	// AttachedType indicates that the block device is hot-plugged into the virtual machine.
	AttachedType Type = "Attached"
)

type (
	// BlockDeviceReadyReason represents the various reasons for the `BlockDeviceReady` condition type.
	BlockDeviceReadyReason = string
	// VirtualMachineReadyReason represents the various reasons for the `VirtualMachineReady` condition type.
	VirtualMachineReadyReason = string
	// AttachedReason represents the various reasons for the `Attached` condition type.
	AttachedReason = string
)

const (
	// BlockDeviceReady signifies that the block device is ready to be hot-plugged, allowing the hot-plug process to start.
	BlockDeviceReady BlockDeviceReadyReason = "BlockDeviceReady"
	// BlockDeviceNotReady signifies that the block device is not ready, preventing the hot-plug process from starting.
	BlockDeviceNotReady BlockDeviceReadyReason = "BlockDeviceNotReady"

	// VirtualMachineReady signifies that the virtual machine is ready for hot-plugging a disk, allowing the hot-plug process to start.
	VirtualMachineReady VirtualMachineReadyReason = "VirtualMachineReady"
	// VirtualMachineNotReady signifies that the virtual machine is not ready, preventing the hot-plug process from starting.
	VirtualMachineNotReady VirtualMachineReadyReason = "VirtualMachineNotReady"

	// Attached signifies that the virtual disk is successfully hot-plugged into the virtual machine.
	Attached AttachedReason = "Attached"
	// NotAttached signifies that the virtual disk is not yet hot-plugged into the virtual machine.
	NotAttached AttachedReason = "NotAttached"
	// AttachmentRequestSent signifies that the attachment request has been sent and the hot-plug process has started.
	AttachmentRequestSent AttachedReason = "AttachmentRequestSent"
	// Conflict indicates that virtual disk is already attached to the virtual machine:
	// Either there is another `VirtualMachineBlockDeviceAttachment` with the same virtual machine and virtual disk to be hot-plugged.
	// or the virtual disk is already attached to the virtual machine spec.
	// Only the one that was created or started sooner can be processed.
	Conflict AttachedReason = "Conflict"
)
