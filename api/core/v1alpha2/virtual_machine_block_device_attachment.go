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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	VirtualMachineBlockDeviceAttachmentKind     = "VirtualMachineBlockDeviceAttachment"
	VirtualMachineBlockDeviceAttachmentResource = "virtualmachineblockdeviceattachments"
)

// VirtualMachineBlockDeviceAttachment provides a hot plug for attaching a disk to a virtual machine.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={virtualization},scope=Namespaced,shortName={vmbda},singular=virtualmachineblockdeviceattachment
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase",description="VirtualMachineBlockDeviceAttachment phase."
// +kubebuilder:printcolumn:name="BLOCKDEVICE KIND",type=string,JSONPath=`.spec.blockDeviceRef.kind`,priority=1,description="Attached blockdevice kind."
// +kubebuilder:printcolumn:name="BLOCKDEVICE NAME",type=string,JSONPath=`.spec.blockDeviceRef.name`,priority=1,description="Attached blockdevice name."
// +kubebuilder:printcolumn:name="VIRTUALMACHINE",type="string",JSONPath=".status.virtualMachineName",description="Name of the virtual machine the disk is attached to."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="Time of resource creation."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineBlockDeviceAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineBlockDeviceAttachmentSpec   `json:"spec"`
	Status VirtualMachineBlockDeviceAttachmentStatus `json:"status,omitempty"`
}

// VirtualMachineBlockDeviceAttachmentList contains a list of VirtualMachineBlockDeviceAttachment resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineBlockDeviceAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs.
	Items []VirtualMachineBlockDeviceAttachment `json:"items"`
}

type VirtualMachineBlockDeviceAttachmentSpec struct {
	// Virtual machine name the disk or image should be attached to.
	// +kubebuilder:validation:MinLength=1
	VirtualMachineName string         `json:"virtualMachineName"`
	BlockDeviceRef     VMBDAObjectRef `json:"blockDeviceRef"`
}

type VirtualMachineBlockDeviceAttachmentStatus struct {
	Phase BlockDeviceAttachmentPhase `json:"phase,omitempty"`
	// Name of the virtual machine the disk is attached to.
	VirtualMachineName string `json:"virtualMachineName,omitempty"`
	// Contains details of the current API resource state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Block device that will be connected to the VM as a hot-plug disk.
type VMBDAObjectRef struct {
	// Block device type. Available options:
	// * `VirtualDisk`: Use VirtualDisk as the disk. This type is always mounted in RW mode.
	// * `VirtualImage`: Use VirtualImage as the disk. This type is always mounted in RO mode.
	// * `ClusterVirtualImage`: Use ClusterVirtualImage as the disk. This type is always mounted in RO mode.
	Kind VMBDAObjectRefKind `json:"kind,omitempty"`
	// Name of the block device to attach.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name,omitempty"`
}

// VMBDAObjectRefKind defines the block device type.
//
// +kubebuilder:validation:Enum={VirtualDisk,VirtualImage,ClusterVirtualImage}
type VMBDAObjectRefKind string

const (
	VMBDAObjectRefKindVirtualDisk         VMBDAObjectRefKind = "VirtualDisk"
	VMBDAObjectRefKindVirtualImage        VMBDAObjectRefKind = "VirtualImage"
	VMBDAObjectRefKindClusterVirtualImage VMBDAObjectRefKind = "ClusterVirtualImage"
)

// BlockDeviceAttachmentPhase defines the current status of the resource:
// * `Pending`: The resource has been created and is on a waiting queue.
// * `InProgress`: The disk is being attached to the VM.
// * `Attached`: The disk has been attached to the VM.
// * `Failed`: There was an error when attaching the disk.
// * `Terminating`: The resource is being deleted.
//
// +kubebuilder:validation:Enum={Pending,InProgress,Attached,Failed,Terminating}
type BlockDeviceAttachmentPhase string

const (
	BlockDeviceAttachmentPhasePending     BlockDeviceAttachmentPhase = "Pending"
	BlockDeviceAttachmentPhaseInProgress  BlockDeviceAttachmentPhase = "InProgress"
	BlockDeviceAttachmentPhaseAttached    BlockDeviceAttachmentPhase = "Attached"
	BlockDeviceAttachmentPhaseFailed      BlockDeviceAttachmentPhase = "Failed"
	BlockDeviceAttachmentPhaseTerminating BlockDeviceAttachmentPhase = "Terminating"
)
