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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	DownloadSpeed *StatusSpeed `json:"downloadSpeed,omitempty"`
	Capacity      string       `json:"capacity,omitempty"`
	Target        DiskTarget   `json:"target"`
	Progress      string       `json:"progress,omitempty"`
	// Deprecated: use ImageUploadURLs instead.
	UploadCommand             string                   `json:"uploadCommand,omitempty"`
	ImageUploadURLs           *ImageUploadURLs         `json:"imageUploadURLs,omitempty"`
	Phase                     DiskPhase                `json:"phase"`
	AttachedToVirtualMachines []AttachedVirtualMachine `json:"attachedToVirtualMachines,omitempty"`
	Stats                     VirtualDiskStats         `json:"stats"`
	SourceUID                 *types.UID               `json:"sourceUID,omitempty"`
	Conditions                []metav1.Condition       `json:"conditions,omitempty"`
	ObservedGeneration        int64                    `json:"observedGeneration,omitempty"`
}

type VirtualDiskStats struct {
	CreationDuration VirtualDiskStatsCreationDuration `json:"creationDuration"`
}

type VirtualDiskStatsCreationDuration struct {
	WaitingForDependencies *metav1.Duration `json:"waitingForDependencies,omitempty"`
	DVCRProvisioning       *metav1.Duration `json:"dvcrProvisioning,omitempty"`
	TotalProvisioning      *metav1.Duration `json:"totalProvisioning,omitempty"`
}

type AttachedVirtualMachine struct {
	Name string `json:"name"`
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
	VirtualDiskObjectRefKindVirtualDiskSnapshot VirtualDiskObjectRefKind = "VirtualDiskSnapshot"
)

type DiskTarget struct {
	PersistentVolumeClaim string `json:"persistentVolumeClaimName,omitempty"`
}

type VirtualDiskPersistentVolumeClaim struct {
	StorageClass *string            `json:"storageClassName,omitempty"`
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
	DiskPending              DiskPhase = "Pending"
	DiskWaitForUserUpload    DiskPhase = "WaitForUserUpload"
	DiskWaitForFirstConsumer DiskPhase = "WaitForFirstConsumer"
	DiskProvisioning         DiskPhase = "Provisioning"
	DiskFailed               DiskPhase = "Failed"
	DiskLost                 DiskPhase = "Lost"
	DiskReady                DiskPhase = "Ready"
	DiskResizing             DiskPhase = "Resizing"
	DiskTerminating          DiskPhase = "Terminating"
)
