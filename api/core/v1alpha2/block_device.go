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

type BlockDeviceSpecRef struct {
	Kind BlockDeviceKind `json:"kind"`
	// The name of attached resource.
	Name string `json:"name"`
	// Boot priority for the block device. 1-based: 1 = first to boot. Smaller value = higher priority.
	// +optional
	// +kubebuilder:validation:Minimum=1
	BootOrder *int `json:"bootOrder,omitempty"`
}

type BlockDeviceStatusRef struct {
	Kind BlockDeviceKind `json:"kind"`
	// The name of attached resource.
	Name string `json:"name"`
	// The size of attached block device.
	Size string `json:"size"`
	// The block device is attached to the virtual machine.
	Attached bool `json:"attached"`
	// The name of attached block device.
	// +kubebuilder:example=sda
	Target string `json:"target,omitempty"`
	// Block device is attached via hot plug connection.
	Hotplugged bool `json:"hotplugged,omitempty"`
	// The name of the `VirtualMachineBlockDeviceAttachment` resource that defines hot plug disk connection to the virtual machine.
	VirtualMachineBlockDeviceAttachmentName string `json:"virtualMachineBlockDeviceAttachmentName,omitempty"`
}

// The BlockDeviceKind is a type of the block device. Options are:
//
// * `ClusterVirtualImage` — Use `ClusterVirtualImage` as the disk. This type is always mounted in RO mode. If the image is an iso-image, it will be mounted as a CDROM device.
// * `VirtualImage` — Use `VirtualImage` as the disk. This type is always mounted in RO mode. If the image is an iso-image, it will be mounted as a CDROM device.
// * `VirtualDisk` — Use `VirtualDisk` as the disk. This type is always mounted in RW mode.
// +kubebuilder:validation:Enum:={ClusterVirtualImage,VirtualImage,VirtualDisk}
type BlockDeviceKind string

const (
	ClusterImageDevice BlockDeviceKind = "ClusterVirtualImage"
	ImageDevice        BlockDeviceKind = "VirtualImage"
	DiskDevice         BlockDeviceKind = "VirtualDisk"
)
