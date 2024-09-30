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
	VirtualMachineRestoreKind     = "VirtualMachineRestore"
	VirtualMachineRestoreResource = "virtualmachinerestores"
)

// VirtualMachineRestore provides a resource that allows to restore a snapshot of the virtual machine and all its resources.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=virtualization,scope=Namespaced,shortName={vmrestore,vmrestores},singular=virtualmachinerestore
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="VirtualMachineRestore phase."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="VirtualMachineRestore age."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineRestoreSpec   `json:"spec"`
	Status VirtualMachineRestoreStatus `json:"status,omitempty"`
}

// VirtualMachineRestoreList contains a list of `VirtualMachineRestore`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineRestore `json:"items"`
}

type VirtualMachineRestoreSpec struct {
	// The name of virtual machine snapshot to restore the virtual machine.
	//
	// +kubebuilder:validation:MinLength=1
	VirtualMachineSnapshotName string `json:"virtualMachineSnapshotName"`
	// Redefining the virtual machine resource names.
	NameReplacements []NameReplacement `json:"nameReplacements,omitempty"`
}

type VirtualMachineRestoreStatus struct {
	Phase VirtualMachineRestorePhase `json:"phase"`
	// Contains details of the current state of this API Resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// The generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NameReplacement represents rule to redefine the virtual machine resource names.
type NameReplacement struct {
	// The selector to choose resources for name replacement.
	From NameReplacementFrom `json:"from"`
	// The new resource name.
	To string `json:"to"`
}

// NameReplacementFrom represents the selector to choose resources for name replacement.
type NameReplacementFrom struct {
	// The kind of resource to rename.
	Kind string `json:"kind,omitempty"`
	// The current name of resource to rename.
	Name string `json:"name"`
}

// VirtualMachineRestorePhase defines current status of resource:
// * Pending - the resource has been created and is on a waiting queue.
// * InProgress - the process of creating the virtual machine from the snapshot is currently underway.
// * Ready - the virtual machine creation from the snapshot has successfully completed.
// * Failed - an error occurred during the virtual machine creation process.
// * Terminating - the resource is in the process of being deleted.
//
// +kubebuilder:validation:Enum={Pending,InProgress,Ready,Failed,Terminating}
type VirtualMachineRestorePhase string

const (
	VirtualMachineRestorePhasePending     VirtualMachineRestorePhase = "Pending"
	VirtualMachineRestorePhaseInProgress  VirtualMachineRestorePhase = "InProgress"
	VirtualMachineRestorePhaseReady       VirtualMachineRestorePhase = "Ready"
	VirtualMachineRestorePhaseFailed      VirtualMachineRestorePhase = "Failed"
	VirtualMachineRestorePhaseTerminating VirtualMachineRestorePhase = "Terminating"
)
