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

// VirtualMachineIPAddress defines IP address for virtual machine.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineIPAddress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineIPAddressSpec   `json:"spec,omitempty"`
	Status VirtualMachineIPAddressStatus `json:"status,omitempty"`
}

// VirtualMachineIPAddressList contains a list of VirtualMachineIPAddress
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineIPAddressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineIPAddress `json:"items"`
}

// VirtualMachineIPAddressSpec is the desired state of `VirtualMachineIPAddress`.
type VirtualMachineIPAddressSpec struct {
	// The issued `VirtualMachineIPAddressLease`, managed automatically.
	VirtualMachineIPAddressLease string `json:"virtualMachineIPAddressLeaseName"`
	// The requested IP address. If omitted the next available IP address will be assigned.
	Address string `json:"address"`
	// Determines the behavior of VirtualMachineIPAddressLease upon VirtualMachineIPAddress deletion.
	ReclaimPolicy VirtualMachineIPAddressReclaimPolicy `json:"reclaimPolicy,omitempty"`
}

// VirtualMachineIPAddressStatus is the observed state of `VirtualMachineIPAddress`.
type VirtualMachineIPAddressStatus struct {
	// Represents the virtual machine that currently uses this IP address.
	VirtualMachine string `json:"virtualMachineName,omitempty"`
	// Assigned IP address.
	Address string `json:"address,omitempty"`
	// The issued `VirtualMachineIPAddressLease`, managed automatically.
	Lease string `json:"virtualMachineIPAddressLeaseName,omitempty"`
	// Represents the current state of IP address.
	Phase VirtualMachineIPAddressPhase `json:"phase,omitempty"`
	// Detailed description of the error.
	ConflictMessage    string             `json:"conflictMessage,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

type VirtualMachineIPAddressPhase string

const (
	VirtualMachineIPAddressPhasePending  VirtualMachineIPAddressPhase = "Pending"
	VirtualMachineIPAddressPhaseBound    VirtualMachineIPAddressPhase = "Bound"
	VirtualMachineIPAddressPhaseLost     VirtualMachineIPAddressPhase = "Lost"
	VirtualMachineIPAddressPhaseConflict VirtualMachineIPAddressPhase = "Conflict"
)
