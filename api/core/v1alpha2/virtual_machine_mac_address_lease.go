/*
Copyright 2025 Flant JSC

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

const (
	VirtualMachineMACAddressLeaseKind     = "VirtualMachineMACAddressLease"
	VirtualMachineMACAddressLeaseResource = "virtualmachinemacaddressleases"
)

// VirtualMachineMACAddressLease defines fact of issued lease for `VirtualMachineMACAddress`.
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineMACAddressLease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineMACAddressLeaseSpec   `json:"spec,omitempty"`
	Status VirtualMachineMACAddressLeaseStatus `json:"status,omitempty"`
}

// VirtualMachineMACAddressLeaseList contains a list of VirtualMachineMACAddressLease
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineMACAddressLeaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineMACAddressLease `json:"items"`
}

// VirtualMachineMACAddressLeaseSpec is the desired state of `VirtualMachineMACAddressLease`.
type VirtualMachineMACAddressLeaseSpec struct {
	// The link to existing `VirtualMachineMACAddress`.
	VirtualMachineMACAddressRef *VirtualMachineMACAddressLeaseMACAddressRef `json:"virtualMachineMACAddressRef,omitempty"`
}

type VirtualMachineMACAddressLeaseMACAddressRef struct {
	// The Namespace of the referenced `VirtualMachineMACAddress`.
	Namespace string `json:"namespace"`
	// The name of the referenced `VirtualMachineMACAddress`.
	Name string `json:"name"`
}

// VirtualMachineMACAddressLeaseStatus is the observed state of `VirtualMachineMACAddressLease`.
type VirtualMachineMACAddressLeaseStatus struct {
	// Represents the current state of issued MAC address lease.
	Phase              VirtualMachineMACAddressLeasePhase `json:"phase,omitempty"`
	ObservedGeneration int64                              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition                 `json:"conditions,omitempty"`
}

type VirtualMachineMACAddressLeasePhase string

const (
	VirtualMachineMACAddressLeasePhaseBound VirtualMachineMACAddressLeasePhase = "Bound"
)
