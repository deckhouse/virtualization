package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// VirtualMachineIPAddressLease defines fact of issued lease for `VirtualMachineIPAddressClaim`.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineIPAddressLease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineIPAddressLeaseSpec   `json:"spec,omitempty"`
	Status VirtualMachineIPAddressLeaseStatus `json:"status,omitempty"`
}

// VirtualMachineIPAddressLeaseList contains a list of VirtualMachineIPAddressLease
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineIPAddressLeaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineIPAddressLease `json:"items"`
}

// VirtualMachineIPAddressLeaseSpec is the desired state of `VirtualMachineIPAddressLease`.
type VirtualMachineIPAddressLeaseSpec struct {
	// The link to existing `VirtualMachineIPAddressClaim`.
	ClaimRef *VirtualMachineIPAddressLeaseClaimRef `json:"claimRef,omitempty"`
	// Determines the behavior of VirtualMachineIPAddressLease upon VirtualMachineIPAddressClaim deletion.
	ReclaimPolicy VirtualMachineIPAddressReclaimPolicy `json:"reclaimPolicy,omitempty"`
}

type VirtualMachineIPAddressLeaseClaimRef struct {
	// The Namespace of the referenced `VirtualMachineIPAddressClaim`.
	Namespace string `json:"namespace"`
	// The name of the referenced `VirtualMachineIPAddressClaim`.
	Name string `json:"name"`
}

type VirtualMachineIPAddressReclaimPolicy string

const (
	VirtualMachineIPAddressReclaimPolicyDelete VirtualMachineIPAddressReclaimPolicy = "Delete"
	VirtualMachineIPAddressReclaimPolicyRetain VirtualMachineIPAddressReclaimPolicy = "Retain"
)

// VirtualMachineIPAddressLeaseStatus is the observed state of `VirtualMachineIPAddressLease`.
type VirtualMachineIPAddressLeaseStatus struct {
	// Represents the current state of issued IP address lease.
	Phase VirtualMachineIPAddressLeasePhase `json:"phase,omitempty"`
}

type VirtualMachineIPAddressLeasePhase string

const (
	VirtualMachineIPAddressLeasePhaseBound    VirtualMachineIPAddressLeasePhase = "Bound"
	VirtualMachineIPAddressLeasePhaseReleased VirtualMachineIPAddressLeasePhase = "Released"
)
