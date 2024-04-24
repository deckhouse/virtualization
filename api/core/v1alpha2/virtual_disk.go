package v1alpha2

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VirtualDiskKind     = "VirtualDisk"
	VirtualDiskResource = "virtualdisks"
)

// VirtualDisk is a disk ready to be bound by a VM
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualDisk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualDiskSpec   `json:"spec"`
	Status VirtualDiskStatus `json:"status,omitempty"`
}

type VirtualDiskSpec struct {
	DataSource            *VirtualDiskDataSource           `json:"dataSource,omitempty"`
	PersistentVolumeClaim VirtualDiskPersistentVolumeClaim `json:"persistentVolumeClaim"`
}

type VirtualDiskStatus struct {
	//ImportDuration string                   `json:"importDuration,omitempty"`
	DownloadSpeed  VirtualDiskDownloadSpeed `json:"downloadSpeed"`
	Capacity       string                   `json:"capacity,omitempty"`
	Target         DiskTarget               `json:"target"`
	Progress       string                   `json:"progress,omitempty"`
	UploadCommand  string                   `json:"uploadCommand,omitempty"`
	Phase          DiskPhase                `json:"phase"`
	FailureReason  string                   `json:"failureReason"`
	FailureMessage string                   `json:"failureMessage"`
}

type VirtualDiskDataSource struct {
	Type           DataSourceType               `json:"type,omitempty"`
	HTTP           *DataSourceHTTP              `json:"http,omitempty"`
	ContainerImage *DataSourceContainerRegistry `json:"containerImage,omitempty"`
	ObjectRef      *VirtualDiskObjectRef        `json:"objectRef,omitempty"`
}

type VirtualDiskObjectRef struct {
	Kind VirtualDiskObjectRefKind `json:"kind,omitempty"`
	Name string                   `json:"name,omitempty"`
}

type VirtualDiskObjectRefKind string

const (
	VirtualDiskObjectRefKindVirtualImage        VirtualDiskObjectRefKind = "VirtualImage"
	VirtualDiskObjectRefKindClusterVirtualImage VirtualDiskObjectRefKind = "ClusterVirtualImage"
)

type VirtualDiskDownloadSpeed struct {
	Avg          string `json:"avg,omitempty"`
	AvgBytes     string `json:"avgBytes,omitempty"`
	Current      string `json:"current,omitempty"`
	CurrentBytes string `json:"currentBytes,omitempty"`
}

type DiskTarget struct {
	PersistentVolumeClaim string `json:"persistentVolumeClaim"`
}

type VirtualDiskPersistentVolumeClaim struct {
	StorageClass *string            `json:"storageClass,omitempty"`
	Size         *resource.Quantity `json:"size,omitempty"`
}

// VirtualDiskList contains a list of VirtualDisk
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualDiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualDisk `json:"items"`
}

type DiskPhase string

const (
	DiskPending           DiskPhase = "Pending"
	DiskWaitForUserUpload DiskPhase = "WaitForUserUpload"
	DiskProvisioning      DiskPhase = "Provisioning"
	DiskReady             DiskPhase = "Ready"
	DiskFailed            DiskPhase = "Failed"
	DiskPVCLost           DiskPhase = "PVCLost"
	DiskUnknown           DiskPhase = "Unknown"
)
