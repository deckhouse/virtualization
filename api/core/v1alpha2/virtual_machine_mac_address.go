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

const (
	VirtualMachineMACAddressKind     = "VirtualMachineMACAddress"
	VirtualMachineMACAddressResource = "virtualmachinemacaddresses"
)

// VirtualMachineMACAddress defines MAC address for virtual machine.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineMACAddress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineMACAddressSpec   `json:"spec,omitempty"`
	Status VirtualMachineMACAddressStatus `json:"status,omitempty"`
}

// VirtualMachineMACAddressList contains a list of VirtualMachineMACAddress
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineMACAddressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineMACAddress `json:"items"`
}

// VirtualMachineMACAddressSpec is the desired state of `VirtualMachineMACAddress`.
type VirtualMachineMACAddressSpec struct {
	// MACAddress is the requested MAC address. If omitted - random generate.
	Address string `json:"address,omitempty"`
}

// VirtualMachineMACAddressStatus is the observed state of `VirtualMachineMACAddress`.
type VirtualMachineMACAddressStatus struct {
	// VirtualMachine represents the virtual machine that currently uses this MAC address.
	// It's the name of the virtual machine instance.
	VirtualMachine string `json:"virtualMachineName,omitempty"`

	// Address is the assigned MAC address allocated to the virtual machine.
	Address string `json:"address,omitempty"`

	// Phase represents the current state of the MAC address.
	// It could indicate whether the MAC address is in use, available, or in any other defined state.
	Phase VirtualMachineMACAddressPhase `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// This is used to identify changes that have been recently observed and handled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the latest available observations of the object's state.
	// They provide detailed status and information, such as whether the MAC address allocation was successful, in progress, etc.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type VirtualMachineMACAddressPhase string

const (
	VirtualMachineMACAddressPhasePending  VirtualMachineMACAddressPhase = "Pending"
	VirtualMachineMACAddressPhaseBound    VirtualMachineMACAddressPhase = "Bound"
	VirtualMachineMACAddressPhaseAttached VirtualMachineMACAddressPhase = "Attached"
)
