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
// +kubebuilder:resource:categories={virtualization,all},scope=Namespaced,shortName={vmop,vmops},singular=virtualmachineoperation
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
type VirtualMachineOperationSpec struct {
	Type VMOPType `json:"type"`
	// Name of the virtual machine the operation is performed for.
	VirtualMachine string `json:"virtualMachineName"`
	// Force execution of an operation.
	// Effect on `Restart` and `Stop`: operation performs immediately.
	// Effect on `Evict` and `Migrate`: enable AutoConverge feature to force migration via CPU throttling.
	Force *bool `json:"force,omitempty"`
}

type VirtualMachineOperationStatus struct {
	Phase VMOPPhase `json:"phase"`
	// The latest detailed observations of the VirtualMachineOperation resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	//  Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
// +kubebuilder:validation:Enum={Restart,Start,Stop,Migrate,Evict}
type VMOPType string

const (
	VMOPTypeRestart VMOPType = "Restart"
	VMOPTypeStart   VMOPType = "Start"
	VMOPTypeStop    VMOPType = "Stop"
	VMOPTypeMigrate VMOPType = "Migrate"
	VMOPTypeEvict   VMOPType = "Evict"
)
