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
	VMCPUKind     = "VirtualMachineCPUModel"
	VMCPUResource = "virtualmachinecpumodels"
)

// VirtualMachineCPUModel an immutable resource describing the processor that will be used in the VM.
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineCPUModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineCPUModelSpec   `json:"spec"`
	Status VirtualMachineCPUModelStatus `json:"status"`
}

// VirtualMachineCPUModelList contains a list of VirtualMachineCPUModel
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineCPUModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs
	Items []VirtualMachineCPUModel `json:"items"`
}

type VirtualMachineCPUModelSpec struct {
	Type     VirtualMachineCPUModelSpecType `json:"type"`
	Model    string                         `json:"model"`
	Features []string                       `json:"features"`
}

type VirtualMachineCPUModelSpecType string

const (
	Host     VirtualMachineCPUModelSpecType = "Host"
	Model    VirtualMachineCPUModelSpecType = "Model"
	Features VirtualMachineCPUModelSpecType = "Features"
)

type VirtualMachineCPUModelStatus struct {
	Features *VirtualMachineCPUModelStatusFeatures `json:"features,omitempty"`
	Nodes    *[]string                             `json:"nodes,omitempty"`
	Phase    VirtualMachineCPUModelStatusPhase     `json:"phase"`
}

type VirtualMachineCPUModelStatusFeatures struct {
	Enabled          []string `json:"enabled"`
	NotEnabledCommon []string `json:"notEnabledCommon"`
}

type VirtualMachineCPUModelStatusPhase string

const (
	VMCPUPhasePending     VirtualMachineCPUModelStatusPhase = "Pending"
	VMCPUPhaseInProgress  VirtualMachineCPUModelStatusPhase = "InProgress"
	VMCPUPhaseReady       VirtualMachineCPUModelStatusPhase = "Ready"
	VMCPUPhaseFailed      VirtualMachineCPUModelStatusPhase = "Failed"
	VMCPUPhaseTerminating VirtualMachineCPUModelStatusPhase = "Terminating"
)
