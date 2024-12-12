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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VirtualMachineClassKind     = "VirtualMachineClass"
	VirtualMachineClassResource = "virtualmachineclasses"
)

// VirtualMachineClass resource describes a cpu requirements, node placement and sizing policy for VM resources.
// A resource cannot be deleted as long as it is used in one of the VMs.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization,backup.deckhouse.io/cluster-config=true}
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories={virtualization},scope=Cluster,shortName={vmc,vmcs,vmclass,vmclasses},singular=virtualmachineclass
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="VirtualMachineClass phase."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time of creation resource."
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineClassSpec   `json:"spec"`
	Status VirtualMachineClassStatus `json:"status,omitempty"`
}

// VirtualMachineClassList contains a list of VirtualMachineClass
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of VirtualMachineClasses
	Items []VirtualMachineClass `json:"items"`
}

type VirtualMachineClassSpec struct {
	NodeSelector NodeSelector `json:"nodeSelector,omitempty"`
	// Tolerations are the same as `spec.tolerations` in the [Pod](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/).
	// These tolerations will be merged with tolerations specified in VirtualMachine resource. VirtualMachine tolerations have higher priority.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +kubebuilder:validation:Required
	CPU            CPU            `json:"cpu"`
	SizingPolicies []SizingPolicy `json:"sizingPolicies,omitempty"`
}

// NodeSelector defines selects the nodes that are targeted to VM scheduling.
type NodeSelector struct {
	// A map of {key,value} pairs.
	// A single {key,value} in the matchLabels map is equivalent to an element of matchExpressions, whose key field is "key", the operator is "In", and the values array contains only "value".
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
	// The name of CPU model. More information about models [here](https://libvirt.org/formatdomain.html#cpu-model-and-topology)
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:example=IvyBridge
	Model string `json:"model,omitempty"`
	// A list of CPU instructions (features) required when type=Features.
	// More information about features [here](https://libvirt.org/formatdomain.html#cpu-model-and-topology)
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:example={mmx, vmx, sse2}
	Features []string `json:"features,omitempty"`
	// Create CPU model based on an intersection CPU features for selected nodes.
	Discovery CpuDiscovery `json:"discovery,omitempty"`
}

type CpuDiscovery struct {
	// A selection of nodes on the basis of which a universal CPU model will be created.
	NodeSelector metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// SizingPolicy define policy for allocating computational resources to VMs.
// It is represented as a list.
// The cores.min - cores.max ranges for different elements of the list must not overlap.
type SizingPolicy struct {
	// Memory sizing policy.
	Memory *SizingPolicyMemory `json:"memory,omitempty"`
	// Allowed values of the `coreFraction` parameter.
	CoreFractions []CoreFractionValue `json:"coreFractions,omitempty"`
	// Allowed values of the `dedicatedCores` parameter.
	DedicatedCores []bool `json:"dedicatedCores,omitempty"`
	// The policy applies for a specified range of the number of CPU cores.
	Cores *SizingPolicyCores `json:"cores,omitempty"`
}

// +kubebuilder:validation:Minimum=1
// +kubebuilder:validation:Maximum=100
type CoreFractionValue int

type SizingPolicyMemory struct {
	MemoryMinMax `json:",inline"`
	// Memory size discretization step. I.e. min=2Gi, max=4Gi, step=1Gi allows to set virtual machine memory size to 2Gi, 3Gi, or 4Gi.
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
	// Minimum cpu core count.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:example=1
	Min int `json:"min"`
	// Maximum cpu core count.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Maximum=1024
	// +kubebuilder:example=10
	Max int `json:"max"`
	// Cpu cores count discretization step. I.e. min=2, max=10, step=4 allows to set virtual machine cpu cores to 2, 6, or 10.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:example=1
	Step int `json:"step,omitempty"`
}

// CPUType defines cpu type, the following options are supported:
// * `Host` - a virtual CPU is used that is as close as possible to the platform node's CPU in terms of instruction set.
// This provides high performance and functionality, as well as compatibility with live migration for nodes with similar processor types.
// For example, VM migration between nodes with Intel and AMD processors will not work.
// This is also true for different generations of processors, as their instruction set is different.
// * `HostPassthrough` - uses the physical CPU of the platform node directly without any modifications.
// When using this class, the guest VM can only be transferred to a target node that has a CPU that exactly matches the CPU of the source node.
// * `Discovery` - create a CPU model based on an intersecton CPU features for selected nodes.
// * `Model` - CPU model name. A CPU model is a named and previously defined set of supported CPU instructions.
// * `Features` - the required set of supported instructions for the CPU.
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
	// A list of nodes that support this CPU model.
	// It is not displayed for the types: `Host`, `HostPassthrough`
	//
	// +kubebuilder:example={node-1, node-2}
	AvailableNodes []string `json:"availableNodes,omitempty"`
	// The maximum amount of free CPU and Memory resources observed among all available nodes.
	// +kubebuilder:example={"maxAllocatableResources: {\"cpu\": 1, \"memory\": \"10Gi\"}"}
	MaxAllocatableResources corev1.ResourceList `json:"maxAllocatableResources,omitempty"`
	// The latest detailed observations of the VirtualMachineClass resource.
	Conditions              []metav1.Condition  `json:"conditions,omitempty"`
	// The generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// CpuFeatures
// Information on CPU features for this model.
// Shown only for types `Features` or `Discovery`.
type CpuFeatures struct {
	//  A list of CPU features for this model.
	//
	// +kubebuilder:example={mmx, vmx, sse2}
	Enabled []string `json:"enabled,omitempty"`
	// A list of unused processor features additionally available for a given group of nodes.
	//
	// +kubebuilder:example={ssse3, vme}
	NotEnabledCommon []string `json:"notEnabledCommon,omitempty"`
}

// VirtualMachineClassPhase defines current status of resource:
// * Pending - resource is not ready, waits until suitable nodes supporting the required CPU model become available.
// * Ready - the resource is ready and available for use.
// * Terminating - the resource is terminating.
//
// +kubebuilder:validation:Enum={Pending,Ready,Terminating}
type VirtualMachineClassPhase string

const (
	ClassPhasePending     VirtualMachineClassPhase = "Pending"
	ClassPhaseReady       VirtualMachineClassPhase = "Ready"
	ClassPhaseTerminating VirtualMachineClassPhase = "Terminating"
)
