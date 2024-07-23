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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VirtualMachineClassKind     = "VirtualMachineClass"
	VirtualMachineClassResource = "virtualmachineclasses"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineClassSpec   `json:"spec"`
	Status VirtualMachineClassStatus `json:"status"`
}

// VirtualMachineClassList contains a list of VirtualMachineClass
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs
	Items []VirtualMachineClass `json:"items"`
}

type VirtualMachineClassSpec struct {
	NodeSelector   NodeSelector   `json:"nodeSelector,omitempty"`
	CPU            CPU            `json:"cpu"`
	SizingPolicies []SizingPolicy `json:"sizingPolicies"`
}

type NodeSelector struct {
	MatchLabels      map[string]string                `json:"matchLabels"`
	MatchExpressions []corev1.NodeSelectorRequirement `json:"matchExpressions"`
}

type CPU struct {
	Type      CPUType              `json:"type"`
	Model     string               `json:"model"`
	Features  []string             `json:"features"`
	Discovery metav1.LabelSelector `json:"discovery"`
}

type SizingPolicy struct {
	Memory         *SizingPolicyMemory `json:"memory"`
	CoreFractions  *int                `json:"coreFractions,omitempty"`
	DedicatedCores bool                `json:"dedicatedCores,omitempty"`
	Cores          *SizingPolicyCores  `json:"cores"`
}
type SizingPolicyMemory struct {
	MinMax `json:",inline"`
	Step   string `json:"step"`
}

type MinMax struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

type SizingPolicyCores struct {
	Min  int `json:"min"`
	Max  int `json:"max"`
	Step int `json:"step"`
}

type CPUType string

const (
	CPUTypeHost            CPUType = "Host"
	CPUTypeHostPassthrough CPUType = "HostPassthrough"
	CPUTypeDiscovery       CPUType = "Discovery"
	CPUTypeModel           CPUType = "Model"
	CPUTypeFeatures        CPUType = "Features"
)

type VirtualMachineClassStatus struct {
	Phase              VirtualMachineClassPhase `json:"phase"`
	CpuFeatures        CpuFeatures              `json:"cpuFeatures"`
	AvailableNodes     []string                 `json:"availableNodes"`
	Conditions         []metav1.Condition       `json:"conditions,omitempty"`
	ObservedGeneration int64                    `json:"observedGeneration,omitempty"`
}

type CpuFeatures struct {
	Enabled          []string `json:"enabled"`
	NotEnabledCommon []string `json:"notEnabledCommon"`
}

type VirtualMachineClassPhase string

const (
	ClassPhasePending     VirtualMachineClassPhase = "Pending"
	ClassPhaseReady       VirtualMachineClassPhase = "Ready"
	ClassPhaseTerminating VirtualMachineClassPhase = "Terminating"
)
