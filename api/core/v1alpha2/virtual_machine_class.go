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
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=virtualization,scope=Cluster,shortName={vmc,vmcs},singular=virtualmachineclass
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

	// Items provides a list of CDIs
	Items []VirtualMachineClass `json:"items"`
}

type VirtualMachineClassSpec struct {
	NodeSelector NodeSelector `json:"nodeSelector,omitempty"`
	// +kubebuilder:validation:Required
	CPU            CPU            `json:"cpu"`
	SizingPolicies []SizingPolicy `json:"sizingPolicies,omitempty"`
}

// NodeSelector defines selects the nodes that are targeted to VM scheduling.
type NodeSelector struct {
	// A map of {key,value} pairs.
	// A single {key,value} in the matchLabels map is equivalent to an element of matchExpressions, whose key field is "key", the operator is "In", and the values array contains only "value".
	// The requirements are ANDed.
	MatchLabels      map[string]string                `json:"matchLabels"`
	MatchExpressions []corev1.NodeSelectorRequirement `json:"matchExpressions"`
}

// CPU defines the requirements for the virtual CPU model.
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
	// Required instructions for the CPU as a list More information about features [here](https://libvirt.org/formatdomain.html#cpu-model-and-topology)
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:example={mmx, vmx, sse2}
	Features []string `json:"features,omitempty"`
	// Create CPU model based on an intersection CPU features for selected nodes.
	Discovery metav1.LabelSelector `json:"discovery,omitempty"`
}

// SizingPolicy define policy for allocating computational resources to VMs.
// It is represented as a list.
// The cores.min - cores.max ranges for different elements of the list must not overlap.
type SizingPolicy struct {
	// Memory sizing policy.
	Memory *SizingPolicyMemory `json:"memory,omitempty"`
	// Allowed values of the `coreFraction` parameter.
	//
	// +kubebuilder:validation:Enum={5,10,20,50,100}
	CoreFractions []int `json:"coreFractions,omitempty"`
	// Allowed values of the `dedicatedCores` parameter.
	DedicatedCores []bool `json:"dedicatedCores,omitempty"`
	// The policy applies for a specified range of the number of CPU cores.
	Cores *SizingPolicyCores `json:"cores,omitempty"`
}

type SizingPolicyMemory struct {
	MemoryMinMax `json:",inline"`
	// Increase step of memory size from min to max.
	//
	// +kubebuilder:example="512Mi"
	Step resource.Quantity `json:"step"`

	// Amount of memory per CPU core.
	PerCore SizingPolicyMemoryPerCore `json:"perCore"`
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
	// Increase step of cpu core count from min to max.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:example=1
	Step int `json:"step,omitempty"`
}

// CPUType defines cpu type, the following options are supported:
// * `Host` - use the virtual CPU closest to the host. This offers maximum functionality and performance, includes crucial guest CPU flags for security, and assesses live migration compatibility.
// * `HostPassthrough` - use the host's physical CPU directly with no modifications. In `HostPassthrough` mode, the guest can only be live-migrated to a target host that matches the source host extremely closely.
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
	CpuFeatures CpuFeatures              `json:"cpuFeatures"`
	// A list of nodes that support this CPU model.
	// It is not displayed for the types: `Host`, `HostPassthrough`
	//
	// +kubebuilder:example={node-1, node-2}
	AvailableNodes []string           `json:"availableNodes"`
	Conditions     []metav1.Condition `json:"conditions,omitempty"`
	// The generation last processed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// CpuFeatures
// Information on CPU features for this model.
// Shown only for types `Features` or `Discovery`.
type CpuFeatures struct {
	//  A list of CPU features for this model.
	//
	// +kubebuilder:example={mmx, vmx, sse2}
	Enabled []string `json:"enabled"`
	// A list of unused processor features additionally available for a given group of nodes.
	//
	// +kubebuilder:example={ssse3, vme}
	NotEnabledCommon []string `json:"notEnabledCommon"`
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
