/*
Copyright 2025 Flant JSC

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
package v1alpha3

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VirtualMachineClassKind     = "VirtualMachineClass"
	VirtualMachineClassResource = "virtualmachineclasses"
)

// VirtualMachineClass resource describes CPU requirements, node placement, and sizing policy for VM resources.
// A resource cannot be deleted as long as it is used in one of the VMs.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization,backup.deckhouse.io/cluster-config=true}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={virtualization-cluster},scope=Cluster,shortName={vmc,vmclass},singular=virtualmachineclass
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="VirtualMachineClass phase."
// +kubebuilder:printcolumn:name="IsDefault",type="string",JSONPath=".metadata.annotations.virtualmachineclass\\.virtualization\\.deckhouse\\.io\\/is-default-class",description="Default class for virtual machines without specified class."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time of resource creation."
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineClassSpec   `json:"spec"`
	Status VirtualMachineClassStatus `json:"status,omitempty"`
}

// VirtualMachineClassList contains a list of VirtualMachineClasses.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of VirtualMachineClasses.
	Items []VirtualMachineClass `json:"items"`
}

type VirtualMachineClassSpec struct {
	NodeSelector NodeSelector `json:"nodeSelector,omitempty"`
	// Tolerations are the same as `spec.tolerations` for [pods](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/).
	// These tolerations will be merged with the tolerations specified in the VirtualMachine resource. VirtualMachine tolerations have a higher priority.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +kubebuilder:validation:Required
	CPU            CPU            `json:"cpu"`
	SizingPolicies []SizingPolicy `json:"sizingPolicies,omitempty"`
}

// NodeSelector defines the nodes targeted for VM scheduling.
type NodeSelector struct {
	// A map of {key,value} pairs.
	// A single {key,value} pair in the matchLabels map is equivalent to an element of matchExpressions whose key field is "key", operator is "In", and the value array contains only "value".
	// The requirements are ANDed.
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	// A list of node selector requirements by node's labels.
	MatchExpressions []corev1.NodeSelectorRequirement `json:"matchExpressions,omitempty"`
}

// CPU defines the requirements for the virtual CPU model.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message=".spec.cpu is immutable"
// +kubebuilder:validation:XValidation:rule="self.type == 'HostPassthrough' || self.type == 'Host' ? !has(self.model) && !has(self.features) && !has(self.discovery) : true",message="HostPassthrough and Host cannot have model, features or discovery"
// +kubebuilder:validation:XValidation:rule="self.type == 'Discovery' ? !has(self.model) && !has(self.features) : true",message="Discovery cannot have model or features"
// +kubebuilder:validation:XValidation:rule="self.type == 'Model' ? has(self.model) && !has(self.features) && !has(self.discovery) : true",message="Model requires model and cannot have features or discovery"
// +kubebuilder:validation:XValidation:rule="self.type == 'Features' ? has(self.features) && !has(self.model) && !has(self.discovery): true",message="Features requires features and cannot have model or discovery"
type CPU struct {
	// +kubebuilder:validation:Required
	Type CPUType `json:"type"`
	// CPU model name. For more information about CPU models and topology, refer to the [libvirt docs](https://libvirt.org/formatdomain.html#cpu-model-and-topology).
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:example=IvyBridge
	Model string `json:"model,omitempty"`
	// List of CPU instructions (features) required when type=Features.
	// For more information about CPU features, refer to the [libvirt docs](https://libvirt.org/formatdomain.html#cpu-model-and-topology).
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:example={mmx, vmx, sse2}
	Features []string `json:"features,omitempty"`
	// Create a CPU model based on intersecting CPU features for selected nodes.
	Discovery *CpuDiscovery `json:"discovery,omitempty"`
}

type CpuDiscovery struct {
	// A selection of nodes to be used as the basis for creating a universal CPU model.
	NodeSelector metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// SizingPolicy defines a policy for allocating computational resources to VMs.
// It is represented as a list.
// The cores.min - cores.max ranges for different elements of the list must not overlap.
type SizingPolicy struct {
	// Memory sizing policy.
	Memory *SizingPolicyMemory `json:"memory,omitempty"`
	// Allowed values of the `coreFraction` parameter in percentages (e.g., "5%", "10%", "25%", "50%", "100%").
	CoreFractions []CoreFractionValue `json:"coreFractions,omitempty"`
	// Allowed values of the `dedicatedCores` parameter.
	DedicatedCores []bool `json:"dedicatedCores,omitempty"`
	// The policy applies for a specified range of the number of CPU cores.
	Cores *SizingPolicyCores `json:"cores,omitempty"`
}

// CoreFractionValue represents CPU core fraction as a percentage string (e.g., "5%", "10%", "25%", "50%", "100%").
// +kubebuilder:validation:XValidation:rule="self.matches('^([1-9]|[1-9][0-9]|100)%$')",message="must be a percentage from 1% to 100% (e.g., \"5%\", \"25%\", \"100%\")"
type CoreFractionValue string

type SizingPolicyMemory struct {
	MemoryMinMax `json:",inline"`
	// Memory size discretization step. For example, the combination of `min=2Gi, `max=4Gi` and `step=1Gi` allows to set the virtual machine memory size to 2Gi, 3Gi, or 4Gi.
	//
	// +kubebuilder:example="512Mi"
	Step resource.Quantity `json:"step,omitempty"`

	// Amount of memory per CPU core.
	PerCore SizingPolicyMemoryPerCore `json:"perCore,omitempty"`
}

type SizingPolicyMemoryPerCore struct {
	MemoryMinMax `json:",inline"`
}

type MemoryMinMax struct {
	// Minimum amount of memory.
	//
	// +kubebuilder:example="1Gi"
	Min resource.Quantity `json:"min,omitempty"`
	// Maximum amount of memory.
	//
	// +kubebuilder:example="8Gi"
	Max resource.Quantity `json:"max,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.max > self.min",message="The maximum must be greater than the minimum"
// +kubebuilder:validation:XValidation:rule="has(self.step) ? self.max > self.step : true",message="The maximum must be greater than the step"
type SizingPolicyCores struct {
	// Minimum number of CPU cores.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:example=1
	Min int `json:"min"`
	// Maximum number of CPU cores.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Maximum=1024
	// +kubebuilder:example=10
	Max int `json:"max"`
	// Discretization step for the CPU core number. For example, the combination of `min=2`, `max=10`, and `step=4` allows to set the number of virtual machine CPU cores to 2, 6, or 10.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:example=1
	Step int `json:"step,omitempty"`
}

// CPUType defines the CPU type, the following options are supported:
// * `Host`: Uses a virtual CPU with an instruction set closely matching the platform node's CPU.
// This provides high performance and functionality, as well as compatibility with "live" migration for nodes with similar processor types.
// For example, VM migration between nodes with Intel and AMD processors will not work.
// This is also true for different CPU generations, as their instruction set is different.
// * `HostPassthrough`: Uses the platform node's physical CPU directly, without any modifications.
// When using this class, the guest VM can only be transferred to a target node with a CPU exactly matching the source node's CPU.
// * `Discovery`: Create a virtual CPU based on instruction sets of physical CPUs for a selected set of nodes.
// * `Model`: CPU model. A CPU model is a named and previously defined set of supported CPU instructions.
// * `Features`: A required set of supported instructions for the CPU.
//
// +kubebuilder:validation:Enum={Host,HostPassthrough,Discovery,Model,Features}
type CPUType string

const (
	CPUTypeHost            CPUType = "Host"
	CPUTypeHostPassthrough CPUType = "HostPassthrough"
	CPUTypeDiscovery       CPUType = "Discovery"
	CPUTypeModel           CPUType = "Model"
	CPUTypeFeatures        CPUType = "Features"
)

type VirtualMachineClassStatus struct {
	Phase       VirtualMachineClassPhase `json:"phase"`
	CpuFeatures CpuFeatures              `json:"cpuFeatures,omitempty"`
	// List of nodes that support this CPU model.
	// It is not displayed for the following types: `Host`, `HostPassthrough`.
	//
	// +kubebuilder:example={node-1, node-2}
	AvailableNodes []string `json:"availableNodes,omitempty"`
	// Maximum amount of free CPU and memory resources observed among all available nodes.
	// +kubebuilder:example={"maxAllocatableResources: {\"cpu\": 1, \"memory\": \"10Gi\"}"}
	MaxAllocatableResources corev1.ResourceList `json:"maxAllocatableResources,omitempty"`
	// The latest detailed observations of the VirtualMachineClass resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// CpuFeatures
// Information on CPU features supported by this model.
// Shown only for `Features` or `Discovery` types.
type CpuFeatures struct {
	//  List of CPU features for this model.
	//
	// +kubebuilder:example={mmx, vmx, sse2}
	Enabled []string `json:"enabled,omitempty"`
	// List of unused processor features additionally available for a given group of nodes.
	//
	// +kubebuilder:example={ssse3, vme}
	NotEnabledCommon []string `json:"notEnabledCommon,omitempty"`
}

// VirtualMachineClassPhase defines the current resource status:
// * `Pending`: The resource is not ready and waits until the suitable nodes supporting the required CPU model are available.
// * `Ready`: The resource is ready and available for use.
// * `Terminating`: The resource is terminating.
//
// +kubebuilder:validation:Enum={Pending,Ready,Terminating}
type VirtualMachineClassPhase string

const (
	ClassPhasePending     VirtualMachineClassPhase = "Pending"
	ClassPhaseReady       VirtualMachineClassPhase = "Ready"
	ClassPhaseTerminating VirtualMachineClassPhase = "Terminating"
)
