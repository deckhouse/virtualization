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
	VirtualMachineOperationKind     = "VirtualMachineOperation"
	VirtualmachineOperationResource = "virtualmachineoperations"
)

// VirtualMachineOperation enables declarative management of virtual machine state changes.
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={virtualization},scope=Namespaced,shortName={vmop},singular=virtualmachineoperation
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="VirtualMachineOperation phase."
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="VirtualMachineOperation type."
// +kubebuilder:printcolumn:name="VirtualMachine",type="string",JSONPath=".spec.virtualMachineName",description="VirtualMachine name."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time of resource creation."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineOperationSpec   `json:"spec"`
	Status VirtualMachineOperationStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message=".spec is immutable"
// +kubebuilder:validation:XValidation:rule="self.type == 'Start' ? !has(self.force) || !self.force : true",message="The `Start` operation cannot be performed forcibly."
// +kubebuilder:validation:XValidation:rule="self.type == 'Migrate' ? !has(self.force) || !self.force : true",message="The `Migrate` operation cannot be performed forcibly."
// +kubebuilder:validation:XValidation:rule="self.type == 'Restore' ? has(self.restore) : true",message="Restore requires restore field."
// +kubebuilder:validation:XValidation:rule="self.type == 'Clone' ? has(self.clone) : true",message="Clone requires clone field."
// +kubebuilder:validation:XValidation:rule="!(has(self.migrate)) || self.type == 'Migrate'",message="spec.migrate can only be set when spec.type is 'Migrate'"
type VirtualMachineOperationSpec struct {
	Type VMOPType `json:"type"`
	// Name of the virtual machine the operation is performed for.
	// +kubebuilder:validation:MinLength=1
	VirtualMachine string `json:"virtualMachineName"`
	// Force execution of an operation.
	//
	// * Effect on `Restart` and `Stop`: operation performs immediately.
	// * Effect on `Evict` and `Migrate`: enable the AutoConverge feature to force migration via CPU throttling if the `PreferSafe` or `PreferForced` policies are used for live migration.
	Force *bool `json:"force,omitempty"`
	// Restore defines the restore operation.
	Restore *VirtualMachineOperationRestoreSpec `json:"restore,omitempty"`
	// Clone defines the clone operation.
	Clone *VirtualMachineOperationCloneSpec `json:"clone,omitempty"`
	// Migrate defines the Migrate operation.
	Migrate *VirtualMachineOperationMigrateSpec `json:"migrate,omitempty"`
}

// VirtualMachineOperationRestoreSpec defines the restore operation.
type VirtualMachineOperationRestoreSpec struct {
	Mode SnapshotOperationMode `json:"mode"`
	// VirtualMachineSnapshotName defines the source of the restore operation.
	// +kubebuilder:validation:MinLength=1
	VirtualMachineSnapshotName string `json:"virtualMachineSnapshotName"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.customization) && ((has(self.customization.namePrefix) && size(self.customization.namePrefix) > 0) || (has(self.customization.nameSuffix) && size(self.customization.nameSuffix) > 0))) || (has(self.nameReplacement) && size(self.nameReplacement) > 0)",message="At least one of customization.namePrefix, customization.nameSuffix, or nameReplacement must be set"
// VirtualMachineOperationCloneSpec defines the clone operation.
type VirtualMachineOperationCloneSpec struct {
	Mode SnapshotOperationMode `json:"mode"`
	// NameReplacement defines rules for renaming resources during cloning.
	// +kubebuilder:validation:XValidation:rule="self.all(nr, has(nr.to) && size(nr.to) >= 1 && size(nr.to) <= 59)",message="Each nameReplacement.to must be between 1 and 59 characters"
	NameReplacement []NameReplacement `json:"nameReplacement,omitempty"`
	// Customization defines customization options for cloning.
	Customization *VirtualMachineOperationCloneCustomization `json:"customization,omitempty"`
}

// VirtualMachineOperationMigrateSpec defines the restore operation.
type VirtualMachineOperationMigrateSpec struct {
	// NodeSelector is a selector which must be true for the virtual machine to fit on a node.
	// Selector which must match a node's labels for the virtual machine to be scheduled on
	// that node.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="!has(self.namePrefix) || (size(self.namePrefix) >= 1 && size(self.namePrefix) <= 59)",message="namePrefix length must be between 1 and 59 characters if set"
// +kubebuilder:validation:XValidation:rule="!has(self.nameSuffix) || (size(self.nameSuffix) >= 1 && size(self.nameSuffix) <= 59)",message="nameSuffix length must be between 1 and 59 characters if set"
// VirtualMachineOperationCloneCustomization defines customization options for cloning.
type VirtualMachineOperationCloneCustomization struct {
	// NamePrefix adds a prefix to resource names during cloning.
	// Applied to VirtualMachine, VirtualDisk, VirtualMachineBlockDeviceAttachment, and Secret resources.
	NamePrefix string `json:"namePrefix,omitempty"`
	// NameSuffix adds a suffix to resource names during cloning.
	// Applied to VirtualMachine, VirtualDisk, VirtualMachineBlockDeviceAttachment, and Secret resources.
	NameSuffix string `json:"nameSuffix,omitempty"`
}

type VirtualMachineOperationStatus struct {
	Phase VMOPPhase `json:"phase"`
	// The latest detailed observations of the VirtualMachineOperation resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	//  Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Resources contains the list of resources that are affected by the snapshot operation.
	Resources []SnapshotResourceStatus `json:"resources,omitempty"`
}

const (
	SnapshotResourceStatusInProgress SnapshotResourceStatusPhase = "InProgress"
	SnapshotResourceStatusCompleted  SnapshotResourceStatusPhase = "Completed"
	SnapshotResourceStatusFailed     SnapshotResourceStatusPhase = "Failed"
)

// Current phase of the resource:
// * `InProgress`: The operation for resource is in progress.
// * `Completed`: The operation for resource has been completed successfully.
// * `Failed`: The operation for resource failed. For details, refer to the `Message` field.
// +kubebuilder:validation:Enum={InProgress,Completed,Failed}
type SnapshotResourceStatusPhase string

// SnapshotResourceStatus defines the resource affected by the operation.
type SnapshotResourceStatus struct {
	// API version of the resource.
	APIVersion string `json:"apiVersion"`
	// Name of the resource.
	Name string `json:"name"`
	// Kind of the resource.
	Kind string `json:"kind"`
	// Status of the resource.
	Status SnapshotResourceStatusPhase `json:"status"`
	// Message about the resource.
	Message string `json:"message"`
}

// VirtualMachineOperationList contains a list of VirtualMachineOperation resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineOperation `json:"items"`
}

// Current phase of the resource:
// * `Pending`: The operation is queued for execution.
// * `InProgress`: The operation is in progress.
// * `Completed`: The operation has been completed successfully.
// * `Failed`: The operation failed. For details, refer to the `conditions` field and events.
// * `Terminating`: The operation is being deleted.
// +kubebuilder:validation:Enum={Pending,InProgress,Completed,Failed,Terminating}
type VMOPPhase string

const (
	VMOPPhasePending     VMOPPhase = "Pending"
	VMOPPhaseInProgress  VMOPPhase = "InProgress"
	VMOPPhaseCompleted   VMOPPhase = "Completed"
	VMOPPhaseFailed      VMOPPhase = "Failed"
	VMOPPhaseTerminating VMOPPhase = "Terminating"
)

// Type of the operation to execute on a virtual machine:
// * `Start`: Start the virtual machine.
// * `Stop`: Stop the virtual machine.
// * `Restart`: Restart the virtual machine.
// * `Migrate`: Migrate the virtual machine to another node where it can be started.
// * `Evict`: Migrate the virtual machine to another node where it can be started.
// * `Restore`: Restore the virtual machine from a snapshot.
// * `Clone`: Clone the virtual machine to a new virtual machine.
// +kubebuilder:validation:Enum={Restart,Start,Stop,Migrate,Evict,Restore,Clone}
type VMOPType string

const (
	VMOPTypeRestart VMOPType = "Restart"
	VMOPTypeStart   VMOPType = "Start"
	VMOPTypeStop    VMOPType = "Stop"
	VMOPTypeMigrate VMOPType = "Migrate"
	VMOPTypeEvict   VMOPType = "Evict"
	VMOPTypeRestore VMOPType = "Restore"
	VMOPTypeClone   VMOPType = "Clone"
)
