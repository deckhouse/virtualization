package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VMIKind     = "VirtualMachineImage"
	VMIResource = "virtualmachineimages"
)

// VirtualMachineImage is an image for virtual machines available in the particular namespace.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineImage struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualMachineImageSpec `json:"spec"`

	Status VirtualMachineImageStatus `json:"status,omitempty"`
}

// VirtualMachineImageList provides the needed parameters
// to do request a list of ClusterVirtualMachineImages from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs
	Items []ClusterVirtualMachineImage `json:"items"`
}

type VirtualMachineImageSpec struct {
	DataSource DataSource `json:"dataSource"`
}

type VirtualMachineImageStatus struct {
	ImageStatus `json:",inline"`
}

func (c *VirtualMachineImage) GetDataSource() DataSource {
	return c.Spec.DataSource
}
