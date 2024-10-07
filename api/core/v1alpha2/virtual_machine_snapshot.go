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

// +kubebuilder:object:generate=true
// +groupName=virtualization.deckhouse.io
package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VirtualMachineSnapshotKind     = "VirtualMachineSnapshot"
	VirtualMachineSnapshotResource = "virtualmachinesnapshots"
)

// VirtualMachineSnapshot provides a resource for creating snapshots of virtual machines.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={all,virtualization},scope=Namespaced,shortName={vmsnapshot,vmsnapshots},singular=virtualmachinesnapshot
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="VirtualMachineSnapshot phase."
// +kubebuilder:printcolumn:name="Consistent",type="boolean",JSONPath=".status.consistent",description="VirtualMachineSnapshot consistency."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="VirtualMachineSnapshot age."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSnapshotSpec   `json:"spec"`
	Status VirtualMachineSnapshotStatus `json:"status,omitempty"`
}

// VirtualMachineSnapshotList contains a list of `VirtualMachineSnapshot`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineSnapshot `json:"items"`
}

type VirtualMachineSnapshotSpec struct {
	// The name of virtual machine to take snapshot.
	//
	// +kubebuilder:validation:MinLength=1
	VirtualMachineName string `json:"virtualMachineName"`
	// Create a snapshot of a virtual machine only if it is possible to freeze the machine through the agent.
	//
	// If value is true, the snapshot of the virtual machine will be taken only in the following scenarios:
	// - the virtual machine is powered off.
	// - the virtual machine with an agent, and the freeze operation was successful.
	//
	// +kubebuilder:default:=true
	RequiredConsistency bool `json:"requiredConsistency"`
	// +kubebuilder:default:="Always"
	KeepIPAddress         KeepIPAddress             `json:"keepIPAddress"`
	VolumeSnapshotClasses []VolumeSnapshotClassName `json:"volumeSnapshotClasses,omitempty"`
}

type VirtualMachineSnapshotStatus struct {
	Phase VirtualMachineSnapshotPhase `json:"phase"`
	// The virtual machine snapshot is consistent.
	Consistent *bool `json:"consistent,omitempty"`
	// The name of underlying `Secret`, created for virtual machine snapshotting.
	VirtualMachineSnapshotSecretName string `json:"virtualMachineSnapshotSecretName,omitempty"`
	// The list of `VirtualDiskSnapshot` names for the snapshots taken from the virtual disks of the associated virtual machine.
	VirtualDiskSnapshotNames []string           `json:"virtualDiskSnapshotNames,omitempty"`
	Conditions               []metav1.Condition `json:"conditions,omitempty"`
	// The generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// VolumeSnapshotClassName defines `StorageClass` and `VolumeSnapshotClass` binding.
type VolumeSnapshotClassName struct {
	// The `StorageClass` name associated with `VolumeSnapshotClass`.
	StorageClassName string `json:"storageClassName"`
	// The name of `VolumeSnapshotClass` to use for virtual disk snapshotting.
	VolumeSnapshotClassName string `json:"volumeSnapshotClassName"`
}

// KeepIPAddress defines whether to save the IP address of the virtual machine or not:
//
// * Always - when creating a snapshot, the virtual machine's IP address will be converted from `Auto` to `Static` and saved.
// * Never - when creating a snapshot, the virtual machine's IP address will not be converted.
//
// +kubebuilder:validation:Enum={Always,Never}
type KeepIPAddress string

const (
	KeepIPAddressAlways KeepIPAddress = "Always"
	KeepIPAddressNever  KeepIPAddress = "Never"
)

// VirtualMachineSnapshotPhase defines current status of resource:
//
// * Pending - the resource has been created and is on a waiting queue.
// * InProgress - the process of creating the snapshot is currently underway.
// * Ready - the snapshot creation has successfully completed, and the virtual machine snapshot is now available.
// * Failed - an error occurred during the snapshotting process.
// * Terminating - the resource is in the process of being deleted.
//
// +kubebuilder:validation:Enum={Pending,InProgress,Ready,Failed,Terminating}
type VirtualMachineSnapshotPhase string

const (
	VirtualMachineSnapshotPhasePending     VirtualMachineSnapshotPhase = "Pending"
	VirtualMachineSnapshotPhaseInProgress  VirtualMachineSnapshotPhase = "InProgress"
	VirtualMachineSnapshotPhaseReady       VirtualMachineSnapshotPhase = "Ready"
	VirtualMachineSnapshotPhaseFailed      VirtualMachineSnapshotPhase = "Failed"
	VirtualMachineSnapshotPhaseTerminating VirtualMachineSnapshotPhase = "Terminating"
)
