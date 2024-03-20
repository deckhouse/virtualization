package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CVMIKind     = "ClusterVirtualMachineImage"
	CVMIResource = "clustervirtualmachineimages"
)

// +genclient:nonNamespaced

// ClusterVirtualMachineImage is a cluster wide available image for virtual machines.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterVirtualMachineImage struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterVirtualMachineImageSpec `json:"spec"`

	Status ClusterVirtualMachineImageStatus `json:"status,omitempty"`
}

// ClusterVirtualMachineImageList provides the needed parameters
// to do request a list of ClusterVirtualMachineImages from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterVirtualMachineImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs
	Items []ClusterVirtualMachineImage `json:"items"`
}

type ClusterVirtualMachineImageSpec struct {
	DataSource CVMIDataSource `json:"dataSource"`
}

type CVMIDataSource struct {
	Type                       DataSourceType               `json:"type,omitempty"`
	HTTP                       *DataSourceHTTP              `json:"http,omitempty"`
	ContainerImage             *DataSourceContainerRegistry `json:"containerImage,omitempty"`
	VirtualMachineImage        *DataSourceNameNamespacedRef `json:"virtualMachineImage,omitempty"`
	ClusterVirtualMachineImage *DataSourceNamedRef          `json:"clusterVirtualMachineImage,omitempty"`
}

type ClusterVirtualMachineImageStatus struct {
	ImageStatus `json:",inline"`
}
