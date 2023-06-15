package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	VMDKind     = "VirtualMachineDisk"
	VMDResource = "virtualmachinedisks"
)

// VirtualMachineDisk is a disk ready to be bound by a VM
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineDisk struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualMachineDiskSpec `json:"spec"`

	Status VirtualMachineDiskStatus `json:"status,omitempty"`
}

type VirtualMachineDiskSpec struct {
	DataSource            DataSource                              `json:"dataSource,omitempty"`
	PersistentVolumeClaim VirtualMachineDiskPersistentVolumeClaim `json:"persistentVolumeClaim"`
}

type VirtualMachineDiskStatus struct{}

type VirtualMachineDiskPersistentVolumeClaim struct {
	Size             string `json:"size"`
	StorageClassName string `json:"storageClassName,omitempty"`
}

// VirtualMachineDiskList contains a list of VirtualMachineDisk
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineDiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineDisk `json:"items"`
}
