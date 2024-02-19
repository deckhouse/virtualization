package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualMachineIPAddressClaim defines IP address claim for virtual machine.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineIPAddressClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineIPAddressClaimSpec   `json:"spec,omitempty"`
	Status VirtualMachineIPAddressClaimStatus `json:"status,omitempty"`
}

// VirtualMachineIPAddressClaimList contains a list of VirtualMachineIPAddressClaim
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineIPAddressClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineIPAddressClaim `json:"items"`
}

// VirtualMachineIPAddressClaimSpec is the desired state of `VirtualMachineIPAddressClaim`.
type VirtualMachineIPAddressClaimSpec struct {
	// The issued `VirtualMachineIPAddressLease`, managed automatically.
	LeaseName string `json:"leaseName"`
	// The requested IP address. If omitted the next available IP address will be assigned.
	Address string `json:"address"`
	// Determines the behavior of VirtualMachineIPAddressLease upon VirtualMachineIPAddressClaim deletion.
	ReclaimPolicy VirtualMachineIPAddressReclaimPolicy `json:"reclaimPolicy,omitempty"`
}

// VirtualMachineIPAddressClaimStatus is the observed state of `VirtualMachineIPAddressClaim`.
type VirtualMachineIPAddressClaimStatus struct {
	// Represents the virtual machine that currently uses this IP address.
	VMName string `json:"virtualMachineName,omitempty"`
	// Assigned IP address.
	Address string `json:"address,omitempty"`
	// The issued `VirtualMachineIPAddressLease`, managed automatically.
	LeaseName string `json:"leaseName,omitempty"`
	// Represents the current state of IP address claim.
	Phase VirtualMachineIPAddressClaimPhase `json:"phase,omitempty"`
	// Detailed description of the error.
	ConflictMessage string `json:"conflictMessage,omitempty"`
}

type VirtualMachineIPAddressClaimPhase string

const (
	VirtualMachineIPAddressClaimPhasePending  VirtualMachineIPAddressClaimPhase = "Pending"
	VirtualMachineIPAddressClaimPhaseBound    VirtualMachineIPAddressClaimPhase = "Bound"
	VirtualMachineIPAddressClaimPhaseLost     VirtualMachineIPAddressClaimPhase = "Lost"
	VirtualMachineIPAddressClaimPhaseConflict VirtualMachineIPAddressClaimPhase = "Conflict"
)
