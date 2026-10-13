/*
Copyright 2026 Flant JSC

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

package sdnapi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Allocation types of an IPAddress.
const (
	// IPAddressTypeAuto takes the next free address of the pool.
	IPAddressTypeAuto = "Auto"
	// IPAddressTypeStatic asks for the address named in spec.static.ip.
	IPAddressTypeStatic = "Static"
)

// Phases of an IPAddress.
const (
	PhasePending   = "Pending"
	PhaseAllocated = "Allocated"
)

// ConditionAllocated is True once the address is taken out of the pool.
const ConditionAllocated = "Allocated"

// ReasonIPAddressIsUsed is the Allocated=False reason of an address somebody else
// has already taken.
const ReasonIPAddressIsUsed = "IPAddressIsUsed"

// ReasonNoFreeIPAddress is the Allocated=False reason of an exhausted pool.
const ReasonNoFreeIPAddress = "NoFreeIPAddress"

const IPAddressKind = "IPAddress"

// IPAddress is an address of a network served by the sdn module.
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type IPAddress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPAddressSpec   `json:"spec"`
	Status IPAddressStatus `json:"status,omitempty"`
}

type IPAddressSpec struct {
	NetworkRef NetworkRef `json:"networkRef"`
	// Type is Auto or Static.
	Type   string           `json:"type,omitempty"`
	Static *StaticIPAddress `json:"static,omitempty"`
}

// NetworkRef points at the network the address belongs to.
type NetworkRef struct {
	Kind string `json:"kind,omitempty"`
	Name string `json:"name,omitempty"`
}

type StaticIPAddress struct {
	IP string `json:"ip,omitempty"`
	// IgnoreLeaseTTLOnAllocation takes the address even while the lease of a
	// deleted resource still holds it.
	IgnoreLeaseTTLOnAllocation bool `json:"ignoreLeaseTTLOnAllocation,omitempty"`
}

type IPAddressStatus struct {
	Address    string             `json:"address,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	Phase      string             `json:"phase,omitempty"`
	// UsedByPods names the pods running on the address, as namespace/name.
	UsedByPods []string `json:"usedByPods,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type IPAddressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []IPAddress `json:"items"`
}

// ClusterNetworkTypePrimaryCNI is the type of the ClusterNetwork that stands for
// the pod network of the cluster.
const ClusterNetworkTypePrimaryCNI = "PrimaryCNI"

// ClusterNetwork is a cluster-wide network served by the sdn module. Only the
// fields this module reads are declared.
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterNetworkSpec   `json:"spec"`
	Status ClusterNetworkStatus `json:"status,omitempty"`
}

type ClusterNetworkSpec struct {
	Type string    `json:"type"`
	IPAM *IPAMSpec `json:"ipam,omitempty"`
}

type ClusterNetworkStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type IPAMSpec struct {
	IPAddressPoolRef *IPAddressPoolRef `json:"ipAddressPoolRef,omitempty"`
}

type IPAddressPoolRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ClusterNetwork `json:"items"`
}

// ClusterIPAddressPoolKind is the kind of a cluster-wide address pool of the sdn module.
const ClusterIPAddressPoolKind = "ClusterIPAddressPool"

// ClusterIPAddressPool is a cluster-wide address pool of the sdn module. Only the
// fields this module sets are declared: the pool is applied server-side, and a
// declared field would be applied too.
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterIPAddressPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterIPAddressPoolSpec `json:"spec"`
}

type ClusterIPAddressPoolSpec struct {
	Pools []IPAddressPoolRange `json:"pools"`
}

type IPAddressPoolRange struct {
	// Network is the CIDR the addresses are taken from.
	Network string `json:"network"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterIPAddressPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ClusterIPAddressPool `json:"items"`
}
