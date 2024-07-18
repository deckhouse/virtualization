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

// VirtualMachineIPAddressLease defines fact of issued lease for `VirtualMachineIPAddressClaim`.
// +genclient
// +genclient:nonNamespaced
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
	Phase              VirtualMachineIPAddressLeasePhase `json:"phase,omitempty"`
	ObservedGeneration int64                             `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition                `json:"conditions,omitempty"`
}

type VirtualMachineIPAddressLeasePhase string

const (
	VirtualMachineIPAddressLeasePhaseBound    VirtualMachineIPAddressLeasePhase = "Bound"
	VirtualMachineIPAddressLeasePhaseReleased VirtualMachineIPAddressLeasePhase = "Released"
)
