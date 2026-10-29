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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	InternalVirtualizationVirtualMachineReceiverKind = "InternalVirtualizationVirtualMachineReceiver"
	VirtualMachineReceiverResource                   = "internalvirtualizationvirtualmachinereceivers"
)

// InternalVirtualizationVirtualMachineReceiver asks this cluster to receive a live migration of a virtual machine from a
// peer cluster. Only the peer creates it, in the namespace of the paired project and with the
// name of the virtual machine. This cluster creates the virtual machine, its disks and its address from it,
// and the UID of the request identifies the migration on both sides.
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={intvirt},scope=Namespaced,shortName={intvirtvmr}
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.reason"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type InternalVirtualizationVirtualMachineReceiver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineReceiverSpec   `json:"spec"`
	Status VirtualMachineReceiverStatus `json:"status,omitempty"`
}

// InternalVirtualizationVirtualMachineReceiverList contains a list of InternalVirtualizationVirtualMachineReceiver.
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type InternalVirtualizationVirtualMachineReceiverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InternalVirtualizationVirtualMachineReceiver `json:"items"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="the spec is immutable"
type VirtualMachineReceiverSpec struct {
	// The VirtualMachine of the source: its labels and annotations and the spec it runs with.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	VirtualMachineTemplate runtime.RawExtension `json:"virtualMachineTemplate"`
	// The VirtualDisks of the virtual machine in the order of its block devices: their names, labels,
	// annotations, StorageClass and size. The disks here are created blank and receive the data.
	// +optional
	VirtualDiskTemplates []runtime.RawExtension `json:"virtualDiskTemplates,omitempty"`
	// The KubeVirt virtual machine instance of the source, spec and status: the receiving virtual machine
	// runs its domain, and the disks here are checked against the size of its block devices.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	InternalVirtualizationVirtualMachineInstance runtime.RawExtension `json:"internalVirtualizationVirtualMachineInstance"`
	// Address of the main network: the virtual machine keeps it in this cluster.
	// +kubebuilder:validation:MinLength=1
	IPAddress string `json:"ipAddress"`
}

// VirtualMachineReceiverPhase is the progress of the receive.
// +kubebuilder:validation:Enum={Pending,Ready,Running,Failed}
type VirtualMachineReceiverPhase string

const (
	// ReceiverPending: this cluster prepares the virtual machine, its disks and its address.
	ReceiverPending VirtualMachineReceiverPhase = "Pending"
	// ReceiverReady: the receiving migration exists, the source may start its migration.
	ReceiverReady VirtualMachineReceiverPhase = "Ready"
	// ReceiverRunning: the virtual machine runs in this cluster as a regular one; the source may delete its
	// virtual machine.
	ReceiverRunning VirtualMachineReceiverPhase = "Running"
	// ReceiverFailed: this cluster does not receive the virtual machine, see the reason.
	ReceiverFailed VirtualMachineReceiverPhase = "Failed"
)

type VirtualMachineReceiverStatus struct {
	Phase VirtualMachineReceiverPhase `json:"phase,omitempty"`
	// Why the receive failed.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message for the user of the source cluster.
	// +optional
	Message string `json:"message,omitempty"`
	// Whether the receiving virtual machine has run in this cluster: after a failure the source learns
	// whether the guest may run here.
	// +optional
	ReceiverStarted bool `json:"receiverStarted,omitempty"`
	// Where the synchronization controller of this cluster listens: the address the migration from the
	// source connects to.
	// +optional
	SynchronizationAddress string `json:"synchronizationAddress,omitempty"`
	// The pin (sha256 of the public key) of the key the synchronization controller of this cluster generated
	// for this migration: the source gives it to its migration, and the migrations trust each other by the pins.
	// +optional
	SyncPin string `json:"syncPin,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
