package v1alpha2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	VMCPUKind     = "VirtualMachineCPUModel"
	VMCPUResource = "virtualmachinecpumodels"
)

// VirtualMachineCPUModel an immutable resource describing the processor that will be used in the VM.
// +genclient
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
	Features VirtualMachineCPUModelStatusFeatures `json:"features,omitempty"`
	Nodes    []string                             `json:"nodes,omitempty"`
}

type VirtualMachineCPUModelStatusFeatures struct {
	Enabled          []string `json:"enabled"`
	NotEnabledCommon []string `json:"notEnabledCommon"`
}
