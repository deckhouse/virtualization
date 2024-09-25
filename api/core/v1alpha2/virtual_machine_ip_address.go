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

type VirtualMachineIPAddressType string

const (
	VirtualMachineIPAddressTypeAuto   VirtualMachineIPAddressType = "Auto"
	VirtualMachineIPAddressTypeStatic VirtualMachineIPAddressType = "Static"
)

// VirtualMachineIPAddressSpec is the desired state of `VirtualMachineIPAddress`.
type VirtualMachineIPAddressSpec struct {
	// Type specifies the mode of IP address assignment. Possible values are "Auto" for automatic IP assignment,
	// or "Static" for assigning a specific IP address.
	Type VirtualMachineIPAddressType `json:"type"`
	// StaticIP is the requested IP address. If omitted the next available IP address will be assigned.
	StaticIP string `json:"staticIP,omitempty"`
}

// VirtualMachineIPAddressStatus is the observed state of `VirtualMachineIPAddress`.
type VirtualMachineIPAddressStatus struct {
	// VirtualMachine represents the virtual machine that currently uses this IP address.
	// It's the name of the virtual machine instance.
	VirtualMachine string `json:"virtualMachineName,omitempty"`

	// Address is the assigned IP address allocated to the virtual machine.
	Address string `json:"address,omitempty"`

	// Phase represents the current state of the IP address.
	// It could indicate whether the IP address is in use, available, or in any other defined state.
	Phase VirtualMachineIPAddressPhase `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// This is used to identify changes that have been recently observed and handled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the object's state.
	// They provide detailed status and information, such as whether the IP address allocation was successful, in progress, etc.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type VirtualMachineIPAddressPhase string

const (
	VirtualMachineIPAddressPhasePending  VirtualMachineIPAddressPhase = "Pending"
	VirtualMachineIPAddressPhaseBound    VirtualMachineIPAddressPhase = "Bound"
	VirtualMachineIPAddressPhaseAttached VirtualMachineIPAddressPhase = "Attached"
)
