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

// VirtualMachineOperation is operation performed on the VirtualMachine.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineOperationSpec   `json:"spec"`
	Status VirtualMachineOperationStatus `json:"status,omitempty"`
}

type VirtualMachineOperationSpec struct {
	Type           VMOPOperation `json:"type"`
	VirtualMachine string        `json:"virtualMachineName"`
	Force          bool          `json:"force,omitempty"`
}

type VirtualMachineOperationStatus struct {
	Phase          VMOPPhase `json:"phase"`
	FailureReason  string    `json:"failureReason,omitempty"`
	FailureMessage string    `json:"failureMessage,omitempty"`
}

// VirtualMachineOperationList contains a list of VirtualMachineOperation
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineOperation `json:"items"`
}

type VMOPPhase string

const (
	VMOPPhasePending    VMOPPhase = "Pending"
	VMOPPhaseInProgress VMOPPhase = "InProgress"
	VMOPPhaseCompleted  VMOPPhase = "Completed"
	VMOPPhaseFailed     VMOPPhase = "Failed"
)

type VMOPOperation string

const (
	VMOPOperationTypeRestart VMOPOperation = "Restart"
	VMOPOperationTypeStart   VMOPOperation = "Start"
	VMOPOperationTypeStop    VMOPOperation = "Stop"
)
