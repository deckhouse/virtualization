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
	VMBDAKind     = "VirtualMachineBlockDeviceAttachment"
	VMBDAResource = "virtualmachineblockdeviceattachments"
)

// VirtualMachineBlockDeviceAttachment is a disk ready to be bound by a VM
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineBlockDeviceAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineBlockDeviceAttachmentSpec   `json:"spec"`
	Status VirtualMachineBlockDeviceAttachmentStatus `json:"status"`
}

// VirtualMachineBlockDeviceAttachmentList contains a list of VirtualMachineBlockDeviceAttachment
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineBlockDeviceAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs
	Items []VirtualMachineBlockDeviceAttachment `json:"items"`
}

type VirtualMachineBlockDeviceAttachmentSpec struct {
	VirtualMachine string         `json:"virtualMachineName"`
	BlockDeviceRef VMBDAObjectRef `json:"blockDeviceRef"`
}

type VMBDAObjectRef struct {
	Kind VMBDAObjectRefKind `json:"kind,omitempty"`
	Name string             `json:"name,omitempty"`
}

type VMBDAObjectRefKind string

const (
	VMBDAObjectRefKindVirtualDisk VMBDAObjectRefKind = "VirtualDisk"
)

type VirtualMachineBlockDeviceAttachmentObjectRefKind string

const BlockDeviceAttachmentTypeVirtualDisk VirtualMachineBlockDeviceAttachmentObjectRefKind = "VirtualDisk"

type VirtualMachineBlockDeviceAttachmentStatus struct {
	VirtualMachine string                     `json:"virtualMachine,omitempty"`
	Phase          BlockDeviceAttachmentPhase `json:"phase,omitempty"`
	FailureReason  string                     `json:"failureReason,omitempty"`
	FailureMessage string                     `json:"failureMessage,omitempty"`
}

type BlockDeviceAttachmentPhase string

const (
	BlockDeviceAttachmentPhaseInProgress BlockDeviceAttachmentPhase = "InProgress"
	BlockDeviceAttachmentPhaseAttached   BlockDeviceAttachmentPhase = "Attached"
	BlockDeviceAttachmentPhaseFailed     BlockDeviceAttachmentPhase = "Failed"
)
