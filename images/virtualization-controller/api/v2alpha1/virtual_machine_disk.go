package v2alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VMDKind     = "VirtualMachineDisk"
	VMDResource = "virtualmachinedisks"
)

// VirtualMachineDisk is a disk ready to be bound by a VM
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineDisk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineDiskSpec   `json:"spec"`
	Status VirtualMachineDiskStatus `json:"status,omitempty"`
}

type VirtualMachineDiskSpec struct {
	DataSource            *DataSource              `json:"dataSource,omitempty"`
	PersistentVolumeClaim VMDPersistentVolumeClaim `json:"persistentVolumeClaim"`
}

type VirtualMachineDiskStatus struct {
	ImportDuration string           `json:"importDuration,omitempty"`
	DownloadSpeed  VMDDownloadSpeed `json:"downloadSpeed,omitempty"`
	Capacity       string           `json:"capacity,omitempty"`
	Target         DiskTarget       `json:"target,omitempty"`
	Progress       string           `json:"progress,omitempty"`
	UploadCommand  string           `json:"uploadCommand,omitempty"`
	Phase          DiskPhase        `json:"phase"`
}

type VMDDownloadSpeed struct {
	Avg     string `json:"avg"`
	Current string `json:"current"`
}

type DiskTarget struct {
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName"`
}

type VMDPersistentVolumeClaim struct {
	StorageClassName string            `json:"storageClassName"`
	Size             resource.Quantity `json:"size"`
}

// VirtualMachineDiskList contains a list of VirtualMachineDisk
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineDiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualMachineDisk `json:"items"`
}

type DiskPhase string

const (
	DiskPending           DiskPhase = "Pending"
	DiskWaitForUserUpload DiskPhase = "WaitForUserUpload"
	DiskProvisioning      DiskPhase = "Provisioning"
	DiskReady             DiskPhase = "Ready"
	DiskFailed            DiskPhase = "Failed"
	DiskNotReady          DiskPhase = "NotReady"
	DiskPVCLost           DiskPhase = "PVCLost"
)
