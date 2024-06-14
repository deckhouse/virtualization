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
	VirtualMachineIPAddressLease string `json:"virtualMachineIPAddressLeaseName"`
	// The requested IP address. If omitted the next available IP address will be assigned.
	Address string `json:"address"`
	// Determines the behavior of VirtualMachineIPAddressLease upon VirtualMachineIPAddressClaim deletion.
	ReclaimPolicy VirtualMachineIPAddressReclaimPolicy `json:"reclaimPolicy,omitempty"`
}

// VirtualMachineIPAddressClaimStatus is the observed state of `VirtualMachineIPAddressClaim`.
type VirtualMachineIPAddressClaimStatus struct {
	// Represents the virtual machine that currently uses this IP address.
	VirtualMachine string `json:"virtualMachineName,omitempty"`
	// Assigned IP address.
	Address string `json:"address,omitempty"`
	// The issued `VirtualMachineIPAddressLease`, managed automatically.
	Lease string `json:"virtualMachineIPAddressLeaseName,omitempty"`
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
