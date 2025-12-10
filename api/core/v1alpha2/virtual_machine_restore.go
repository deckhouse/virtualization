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

// VirtualMachineRestore provides a resource for restoring a virtual machine and all associated resources from a snapshot.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=virtualization,scope=Namespaced,shortName={vmrestore},singular=virtualmachinerestore
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

// VirtualMachineRestoreList contains a list of VirtualMachineRestore resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineRestore `json:"items"`
}

type VirtualMachineRestoreSpec struct {
	// Virtual machine restore mode:
	//
	// * Safe — in this mode, the virtual machine will not be restored if unresolvable conflicts are detected during the restoration process.
	// * Forced — in this mode, the virtual machine configuration will be updated and all associated resources will be recreated. The virtual machine may malfunction if the recovery process fails. Use the mode when you need to restore the virtual machine despite conflicts.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Safe;Forced
	// +kubebuilder:default:=Safe
	RestoreMode RestoreMode `json:"restoreMode,omitempty"`
	// Snapshot name to restore a virtual machine from.
	//
	// +kubebuilder:validation:MinLength=1
	VirtualMachineSnapshotName string `json:"virtualMachineSnapshotName"`
	// Renaming conventions for virtual machine resources.
	NameReplacements []NameReplacement `json:"nameReplacements,omitempty"`
}

type VirtualMachineRestoreStatus struct {
	Phase VirtualMachineRestorePhase `json:"phase"`
	// Contains details of the current API resource state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NameReplacement represents a rule for redefining the virtual machine resource names.
type NameReplacement struct {
	// Selector to choose resources for name replacement.
	From NameReplacementFrom `json:"from"`
	// New resource name.
	// +kubebuilder:validation:MinLength=1
	To string `json:"to"`
}

// NameReplacementFrom represents a selector to choose resources for name replacement.
type NameReplacementFrom struct {
	// Kind of a resource to rename.
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind,omitempty"`
	// Current name of a resource to rename.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// VirtualMachineRestorePhase defines the current status of a resource:
// * `Pending`: The resource has been created and is on a waiting queue.
// * `InProgress`: A virtual machine is being restored from a snapshot.
// * `Ready`: A virtual machine has been successfully restored from a snapshot.
// * `Failed`: An error occurred when restoring a virtual machine from a snapshot.
// * `Terminating`: The resource is being deleted.
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

type RestoreMode string

const (
	RestoreModeSafe   RestoreMode = "Safe"
	RestoreModeForced RestoreMode = "Forced"
)
