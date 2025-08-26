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
type VirtualMachineOperationSpec struct {
	Type VMOPType `json:"type"`
	// Name of the virtual machine the operation is performed for.
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
}

// VirtualMachineOperationRestoreSpec defines the restore operation.
type VirtualMachineOperationRestoreSpec struct {
	Mode VMOPRestoreMode `json:"mode"`
	// VirtualMachineSnapshotName defines the source of the restore operation.
	VirtualMachineSnapshotName string `json:"virtualMachineSnapshotName"`
}

// VirtualMachineOperationCloneSpec defines the clone operation.
type VirtualMachineOperationCloneSpec struct {
	Mode VMOPRestoreMode `json:"mode"`
	// NameReplacement defines rules for renaming resources during cloning.
	NameReplacement []NameReplacement `json:"nameReplacement,omitempty"`
	// Customization defines customization options for cloning.
	Customization *VirtualMachineOperationCloneCustomization `json:"customization,omitempty"`
}

// VirtualMachineOperationCloneCustomization defines customization options for cloning.
type VirtualMachineOperationCloneCustomization struct {
	// NamePrefix adds a prefix to resource names during cloning.
	// Applied to VirtualDisk, VirtualMachineIPAddress, VirtualMachineMACAddress, and Secret resources.
	NamePrefix string `json:"namePrefix,omitempty"`
	// NameSuffix adds a suffix to resource names during cloning.
	// Applied to VirtualDisk, VirtualMachineIPAddress, VirtualMachineMACAddress, and Secret resources.
	NameSuffix string `json:"nameSuffix,omitempty"`
}

// VMOPRestoreMode defines the kind of the restore operation.
// * `DryRun`: DryRun run without any changes. Compatibility shows in status.
// * `Strict`: Strict restore as is in the snapshot.
// * `BestEffort`: BestEffort restore without deleted external missing dependencies.
// +kubebuilder:validation:Enum={DryRun,Strict,BestEffort}
type VMOPRestoreMode string

const (
	VMOPRestoreModeDryRun     VMOPRestoreMode = "DryRun"
	VMOPRestoreModeStrict     VMOPRestoreMode = "Strict"
	VMOPRestoreModeBestEffort VMOPRestoreMode = "BestEffort"
)

type VirtualMachineOperationStatus struct {
	Phase VMOPPhase `json:"phase"`
	// The latest detailed observations of the VirtualMachineOperation resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	//  Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Resources contains the list of resources that are affected by the snapshot operation.
	Resources []VirtualMachineOperationResource `json:"resources,omitempty"`
}

// VMOPResourceKind defines the kind of the resource affected by the operation.
// * `VirtualDisk`: VirtualDisk resource.
// * `VirtualMachine`: VirtualMachine resource.
// * `VirtualImage`: VirtualImage resource.
// * `ClusterVirtualImage`: ClusterVirtualImage resource.
// * `VirtualMachineIPAddress`: VirtualMachineIPAddress resource.
// * `VirtualMachineIPAddressLease`: VirtualMachineIPAddressLease resource.
// * `VirtualMachineClass`: VirtualMachineClass resource.
// * `VirtualMachineOperation`: VirtualMachineOperation resource.
// +kubebuilder:validation:Enum={VMOPResourceSecret,VMOPResourceNetwork,VMOPResourceVirtualDisk,VMOPResourceVirtualImage,VMOPResourceVirtualMachine,VMOPResourceClusterNetwork,VMOPResourceClusterVirtualImage,VMOPResourceVirtualMachineIPAddress,VMOPResourceVirtualMachineMacAddress,VMOPResourceVirtualMachineBlockDeviceAttachment}
type VMOPResourceKind string

const (
	VMOPResourceSecret                              VMOPResourceKind = "Secret"
	VMOPResourceNetwork                             VMOPResourceKind = "Network"
	VMOPResourceVirtualDisk                         VMOPResourceKind = "VirtualDisk"
	VMOPResourceVirtualImage                        VMOPResourceKind = "VirtualImage"
	VMOPResourceVirtualMachine                      VMOPResourceKind = "VirtualMachine"
	VMOPResourceClusterNetwork                      VMOPResourceKind = "ClusterNetwork"
	VMOPResourceClusterVirtualImage                 VMOPResourceKind = "ClusterVirtualImage"
	VMOPResourceVirtualMachineIPAddress             VMOPResourceKind = "VirtualMachineIPAddress"
	VMOPResourceVirtualMachineMacAddress            VMOPResourceKind = "VirtualMachineMacAddress"
	VMOPResourceVirtualMachineBlockDeviceAttachment VMOPResourceKind = "VirtualMachineBlockDeviceAttachment"
)

const (
	VMOPResourceStatusInProgress VMOPResourceStatusPhase = "InProgress"
	VMOPResourceStatusCompleted  VMOPResourceStatusPhase = "Completed"
	VMOPResourceStatusFailed     VMOPResourceStatusPhase = "Failed"
)

// Current phase of the resource:
// * `InProgress`: The operation for resource is in progress.
// * `Completed`: The operation for resource has been completed successfully.
// * `Failed`: The operation for resource failed. For details, refer to the `Message` field.
// +kubebuilder:validation:Enum={InProgress,Completed,Failed}
type VMOPResourceStatusPhase string

// VirtualMachineOperationResource defines the resource affected by the operation.
type VirtualMachineOperationResource struct {
	// API version of the resource.
	APIVersion string `json:"apiVersion"`
	// Name of the resource.
	Name string `json:"name"`
	// Kind of the resource.
	Kind string `json:"kind"`
	// Status of the resource.
	Status VMOPResourceStatusPhase `json:"status"`
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
// * `Migrate` (deprecated): Migrate the virtual machine to another node where it can be started.
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
