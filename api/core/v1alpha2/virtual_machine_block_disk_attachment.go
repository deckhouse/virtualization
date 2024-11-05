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

// VirtualMachineBlockDeviceAttachment provides a hot plug for connecting a disk to a virtual machine.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={all,virtualization},scope=Namespaced,shortName={vmbda,vmbdas},singular=virtualmachineblockdeviceattachment
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase",description="VirtualMachineBlockDeviceAttachment phase."
// +kubebuilder:printcolumn:name="VIRTUAL MACHINE NAME",type="string",JSONPath=".status.virtualMachineName",description="The name of the virtual machine to which this disk is attached."
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp",description="Time of creation resource."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineBlockDeviceAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineBlockDeviceAttachmentSpec   `json:"spec"`
	Status VirtualMachineBlockDeviceAttachmentStatus `json:"status,omitempty"`
}

// VirtualMachineBlockDeviceAttachmentList contains a list of VirtualMachineBlockDeviceAttachment.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineBlockDeviceAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs
	Items []VirtualMachineBlockDeviceAttachment `json:"items"`
}

type VirtualMachineBlockDeviceAttachmentSpec struct {
	// The name of the virtual machine to which the disk or image should be connected.
	VirtualMachineName string         `json:"virtualMachineName"`
	BlockDeviceRef     VMBDAObjectRef `json:"blockDeviceRef"`
}

type VirtualMachineBlockDeviceAttachmentStatus struct {
	Phase BlockDeviceAttachmentPhase `json:"phase,omitempty"`
	// The name of the virtual machine to which this disk is attached.
	VirtualMachineName string `json:"virtualMachineName,omitempty"`
	// Contains details of the current state of this API Resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// The generation last processed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// A block device that will be connected to the VM as a hot Plug disk.
type VMBDAObjectRef struct {
	// The type of the block device. Options are:
	// * `VirtualDisk` — use `VirtualDisk` as the disk. This type is always mounted in RW mode.
	Kind VMBDAObjectRefKind `json:"kind,omitempty"`
	// The name of block device to attach.
	Name string `json:"name,omitempty"`
}

// VMBDAObjectRefKind defines the type of the block device.
//
// +kubebuilder:validation:Enum={VirtualDisk}
type VMBDAObjectRefKind string

const (
	VMBDAObjectRefKindVirtualDisk VMBDAObjectRefKind = "VirtualDisk"
)

// BlockDeviceAttachmentPhase defines current status of resource:
// * Pending — the resource has been created and is on a waiting queue.
// * InProgress — the disk is in the process of being attached.
// * Attached — the disk is attached to virtual machine.
// * Failed — there was a problem with attaching the disk.
// * Terminating — the process of resource deletion is in progress.
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
