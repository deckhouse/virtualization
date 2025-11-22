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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	VirtualMachineSnapshotOperationKind     = "VirtualMachineSnapshotOperation"
	VirtualmachineSnapshotOperationResource = "virtualmachinesnapshotoperations"
)

// VirtualMachineSnapshotOperation enables declarative management of virtual machine snapshot state changes.
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={virtualization},scope=Namespaced,shortName={vmsop},singular=virtualmachinesnapshotoperation
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="VirtualMachineSnapshotOperation phase."
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="VirtualMachineSnapshotOperation type."
// +kubebuilder:printcolumn:name="VirtualMachineSnapshot",type="string",JSONPath=".spec.virtualMachineSnapshotName",description="VirtualMachineSnapshot name."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time of resource creation."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineSnapshotOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSnapshotOperationSpec   `json:"spec"`
	Status VirtualMachineSnapshotOperationStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message=".spec is immutable"
// +kubebuilder:validation:XValidation:rule="self.type == 'CreateVirtualMachineName' ? has(self.createVirtualMachine) : true",message="CreateVirtualMachineName requires clone field."
type VirtualMachineSnapshotOperationSpec struct {
	Type VMSOPType `json:"type"`
	// Name of the virtual machine snapshot the operation is performed for.
	VirtualMachineSnapshotName string `json:"virtualMachineSnapshotName"`
	// CreateVirtualMachine defines the clone operation.
	CreateVirtualMachine *VMSOPCreateVirtualMachineSpec `json:"createVirtualMachine,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.customization) && ((has(self.customization.namePrefix) && size(self.customization.namePrefix) > 0) || (has(self.customization.nameSuffix) && size(self.customization.nameSuffix) > 0))) || (has(self.nameReplacement) && size(self.nameReplacement) > 0)",message="At least one of customization.namePrefix, customization.nameSuffix, or nameReplacement must be set"
// VMSOPCreateVirtualMachineSpec defines the clone operation.
type VMSOPCreateVirtualMachineSpec struct {
	Mode SnapshotOperationMode `json:"mode"`
	// NameReplacement defines rules for renaming resources during cloning.
	// +kubebuilder:validation:XValidation:rule="self.all(nr, has(nr.to) && size(nr.to) >= 1 && size(nr.to) <= 59)",message="Each nameReplacement.to must be between 1 and 59 characters"
	NameReplacement []NameReplacement `json:"nameReplacement,omitempty"`
	// Customization defines customization options for cloning.
	Customization *VMSOPCreateVirtualMachineCustomization `json:"customization,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="!has(self.namePrefix) || (size(self.namePrefix) >= 1 && size(self.namePrefix) <= 59)",message="namePrefix length must be between 1 and 59 characters if set"
// +kubebuilder:validation:XValidation:rule="!has(self.nameSuffix) || (size(self.nameSuffix) >= 1 && size(self.nameSuffix) <= 59)",message="nameSuffix length must be between 1 and 59 characters if set"
// VMSOPCreateVirtualMachineCustomization defines customization options for cloning.
type VMSOPCreateVirtualMachineCustomization struct {
	// NamePrefix adds a prefix to resource names during cloning.
	// Applied to VirtualMachine, VirtualDisk, VirtualMachineBlockDeviceAttachment, and Secret resources.
	NamePrefix string `json:"namePrefix,omitempty"`
	// NameSuffix adds a suffix to resource names during cloning.
	// Applied to VirtualMachine, VirtualDisk, VirtualMachineBlockDeviceAttachment, and Secret resources.
	NameSuffix string `json:"nameSuffix,omitempty"`
}

type VirtualMachineSnapshotOperationStatus struct {
	Phase VMSOPPhase `json:"phase"`
	// The latest detailed observations of the VirtualMachineSnapshotOperation resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	//  Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Resources contains the list of resources that are affected by the snapshot operation.
	Resources []SnapshotResourceStatus `json:"resources,omitempty"`
}


// VirtualMachineSnapshotOperationList contains a list of VirtualMachineSnapshotOperation resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineSnapshotOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineSnapshotOperation `json:"items"`
}

// Current phase of the resource:
// * `Pending`: The operation is queued for execution.
// * `InProgress`: The operation is in progress.
// * `Completed`: The operation has been completed successfully.
// * `Failed`: The operation failed. For details, refer to the `conditions` field and events.
// * `Terminating`: The operation is being deleted.
// +kubebuilder:validation:Enum={Pending,InProgress,Completed,Failed,Terminating}
type VMSOPPhase string

const (
	VMSOPPhasePending     VMSOPPhase = "Pending"
	VMSOPPhaseInProgress  VMSOPPhase = "InProgress"
	VMSOPPhaseCompleted   VMSOPPhase = "Completed"
	VMSOPPhaseFailed      VMSOPPhase = "Failed"
	VMSOPPhaseTerminating VMSOPPhase = "Terminating"
)

// Type of the operation to execute on a virtual machine:
// * `CreateVirtualMachine`: CreateVirtualMachine the virtual machine to a new virtual machine.
// +kubebuilder:validation:Enum={CreateVirtualMachine}
type VMSOPType string

const (
	VMSOPTypeCreateVirtualMachine VMSOPType = "CreateVirtualMachine"
)
