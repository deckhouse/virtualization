package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	VMName      string                           `json:"virtualMachineName"`
	BlockDevice BlockDeviceAttachmentBlockDevice `json:"blockDevice"`
}

type BlockDeviceAttachmentBlockDevice struct {
	Type               BlockDeviceAttachmentType                `json:"type"`
	VirtualMachineDisk *BlockDeviceAttachmentVirtualMachineDisk `json:"virtualMachineDisk"`
}

type BlockDeviceAttachmentType string

const BlockDeviceAttachmentTypeVirtualMachineDisk BlockDeviceAttachmentType = "VirtualMachineDisk"

type BlockDeviceAttachmentVirtualMachineDisk struct {
	Name string `json:"name"`
}

type VirtualMachineBlockDeviceAttachmentStatus struct {
	VMName         string                     `json:"virtualMachineName,omitempty"`
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
