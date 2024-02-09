package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	VMOPKind     = "VirtualMachineOperation"
	VMOPResource = "virtualmachineoperations"
)

// VirtualMachineOperation is operation performed on the VirtualMachine.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineOperationSpec   `json:"spec"`
	Status VirtualMachineOperationStatus `json:"status,omitempty"`
}

type VirtualMachineOperationSpec struct {
	Type               VMOPOperation `json:"type"`
	VirtualMachineName string        `json:"virtualMachineName"`
	Force              bool          `json:"force,omitempty"`
}

type VirtualMachineOperationStatus struct {
	Phase          VMOPPhase `json:"phase"`
	FailureReason  string    `json:"failureReason,omitempty"`
	FailureMessage string    `json:"failureMessage,omitempty"`
}

// VirtualMachineOperationList contains a list of VirtualMachineOperation
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineOperation `json:"items"`
}

type VMOPPhase string

const (
	VMOPPhasePending    VMOPPhase = "Pending"
	VMOPPhaseInProgress VMOPPhase = "InProgress"
	VMOPPhaseCompleted  VMOPPhase = "Completed"
	VMOPPhaseFailed     VMOPPhase = "Failed"
)

type VMOPOperation string

const (
	VMOPOperationTypeRestart VMOPOperation = "Restart"
	VMOPOperationTypeStart   VMOPOperation = "Start"
	VMOPOperationTypeStop    VMOPOperation = "Stop"
)
