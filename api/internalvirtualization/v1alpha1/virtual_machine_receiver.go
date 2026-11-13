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
	"k8s.io/apimachinery/pkg/types"
)

const (
	InternalVirtualizationVirtualMachineReceiverKind = "InternalVirtualizationVirtualMachineReceiver"
	VirtualMachineReceiverResource                   = "internalvirtualizationvirtualmachinereceivers"
)

// InternalVirtualizationVirtualMachineReceiver is a request to receive a live migration of a virtual machine
// from another site. Only the source site creates it, in the namespace of the paired project and with the
// name of the virtual machine. From this request, this cluster creates the virtual machine, its disks and its
// IP address. The UID of the request identifies the migration on both sites.
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

// MigrationSynchronizationEndpoint is the address and the public key pin of a synchronization controller.
// The endpoint is updated when the controller restarts.
type MigrationSynchronizationEndpoint struct {
	// Address of the synchronization controller, including its port.
	// +optional
	SyncAddress string `json:"syncAddress,omitempty"`
	// SHA-256 pin of the public key that the synchronization controller generated for this migration.
	// +optional
	SyncPin string `json:"syncPin,omitempty"`
}

type VirtualMachineReceiverSpec struct {
	// Name of the ClusterPeer resource that identifies the source site in the receiving cluster.
	// Set by the admission webhook from the authenticated identity of the source site.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="sourcePeer is immutable"
	SourcePeer string `json:"sourcePeer,omitempty"`
	// Synchronization endpoint of the source site. It may change after a restart of the synchronization
	// controller there.
	// +optional
	SourceSynchronization MigrationSynchronizationEndpoint `json:"sourceSynchronization,omitempty"`
	// VirtualMachine of the source site: its labels and annotations and the specification it runs with.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="the receive template is immutable"
	VirtualMachineTemplate runtime.RawExtension `json:"virtualMachineTemplate"`
	// VirtualDisks of the virtual machine in the order of its block devices: their names, labels, annotations,
	// StorageClass and size. This cluster creates the disks blank, and they receive the data.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="the receive template is immutable"
	VirtualDiskTemplates []runtime.RawExtension `json:"virtualDiskTemplates,omitempty"`
	// KubeVirt virtual machine instance of the source site, its spec and status. The receiving virtual machine
	// runs its domain, and the disks of this cluster are checked against the size of its block devices.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="the receive template is immutable"
	InternalVirtualizationVirtualMachineInstance runtime.RawExtension `json:"internalVirtualizationVirtualMachineInstance"`
	// IP address of the virtual machine in the main network. The virtual machine keeps it in this cluster.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="the receive template is immutable"
	IPAddress string `json:"ipAddress"`
}

// VirtualMachineReceiverPhase is the phase of the receive.
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
	// Phase of the receive:
	//
	// * `Pending`: This cluster prepares the virtual machine, its disks and its IP address.
	// * `Ready`: The receiving migration exists, and the source site may start its migration.
	// * `Running`: The virtual machine runs in this cluster as a regular one, and the source site may delete its copy.
	// * `Failed`: This cluster does not receive the virtual machine. The reason is in the `reason` field.
	Phase VirtualMachineReceiverPhase `json:"phase,omitempty"`
	// Reason of the failed receive.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message for the user of the source site: why the receive failed or what holds it up.
	// +optional
	Message string `json:"message,omitempty"`
	// Whether the received virtual machine has run in this cluster. After a failure, the source site learns
	// from it whether the guest may run here.
	// +optional
	ReceiverStarted bool `json:"receiverStarted,omitempty"`
	// Synchronization endpoint of this cluster. The source site reads it for its migration.
	// +optional
	Synchronization MigrationSynchronizationEndpoint `json:"synchronization,omitempty"`
	// Time when the receive entered the `Running` or `Failed` phase. The time to live of the request counts
	// from it. A later transition from `Failed` to `Running` records a new time.
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
	// UID of the virtual machine this cluster created for the request.
	// +optional
	VirtualMachineUID types.UID `json:"virtualMachineUID,omitempty"`
	// UID of the KubeVirt virtual machine this cluster created for the request.
	// +optional
	InternalVirtualMachineUID types.UID `json:"internalVirtualMachineUID,omitempty"`
	// UIDs of the virtual disks this cluster created for the request, by disk name.
	// +optional
	VirtualDiskUIDs map[string]types.UID `json:"virtualDiskUIDs,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
