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
