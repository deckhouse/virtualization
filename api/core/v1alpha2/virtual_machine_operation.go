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
	VMOPKind     = "VirtualMachineOperation"
	VMOPResource = "virtualmachineoperations"
)

// VirtualMachineOperation resource provides the ability to declaratively manage state changes of virtual machines.
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={virtualization,all},scope=Namespaced,shortName={vmop,vmops},singular=virtualmachineoperation
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="VirtualMachineOperation phase."
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="VirtualMachineOperation type."
// +kubebuilder:printcolumn:name="VirtualMachine",type="string",JSONPath=".spec.virtualMachineName",description="VirtualMachine name."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time of creation resource."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineOperationSpec   `json:"spec"`
	Status VirtualMachineOperationStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'Start' ? !has(self.force) || !self.force : true",message="The `Start` operation cannot be performed forcibly."
// +kubebuilder:validation:XValidation:rule="self.type == 'Migrate' ? !has(self.force) || !self.force : true",message="The `Migrate` operation cannot be performed forcibly."
type VirtualMachineOperationSpec struct {
	Type VMOPType `json:"type"`
	// The name of the virtual machine for which the operation is performed.
	VirtualMachine string `json:"virtualMachineName"`
	// Force the execution of the operation. Applies only for Restart and Stop. In this case, the action on the virtual machine is performed immediately.
	Force bool `json:"force,omitempty"`
}

type VirtualMachineOperationStatus struct {
	Phase VMOPPhase `json:"phase"`
	// The latest detailed observations of the VirtualMachineOperation resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	//  The generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// VirtualMachineOperationList contains a list of VirtualMachineOperation
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineOperation `json:"items"`
}

// Represents the current phase of resource:
// * Pending - the operation is queued for execution.
// * InProgress - operation in progress.
// * Completed - the operation was successful.
// * Failed - the operation failed. Check conditions and events for more information.
// * Terminating - the operation is deleted.
// +kubebuilder:validation:Enum={Pending,InProgress,Completed,Failed,Terminating}
type VMOPPhase string

const (
	VMOPPhasePending     VMOPPhase = "Pending"
	VMOPPhaseInProgress  VMOPPhase = "InProgress"
	VMOPPhaseCompleted   VMOPPhase = "Completed"
	VMOPPhaseFailed      VMOPPhase = "Failed"
	VMOPPhaseTerminating VMOPPhase = "Terminating"
)

// Type is operation over the virtualmachine:
// * Start - start the virtualmachine.
// * Stop - stop the virtualmachine.
// * Restart - restart the virtualmachine.
// * Migrate (deprecated) - migrate the virtualmachine.
// * Evict - evict the virtualmachine.
// +kubebuilder:validation:Enum={Restart,Start,Stop,Migrate,Evict}
type VMOPType string

const (
	VMOPTypeRestart VMOPType = "Restart"
	VMOPTypeStart   VMOPType = "Start"
	VMOPTypeStop    VMOPType = "Stop"
	VMOPTypeMigrate VMOPType = "Migrate"
	VMOPTypeEvict   VMOPType = "Evict"
)
