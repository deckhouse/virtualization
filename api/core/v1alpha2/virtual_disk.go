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

// The `VirtualDisk` resource describes the desired virtual machine disk configuration. A `VirtualDisk` can be mounted statically in the virtual machine by specifying it in the `.spec.blockDeviceRefs` disk list, or mounted on-the-fly using the `VirtualMachineBlockDeviceAttachments` resource.
//
// Once `VirtualDisk` is created, only the disk size `.spec.persistentVolumeClaim.size` can be changed, all other fields are immutable.
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization,all},scope=Namespaced,shortName={vd,vds},singular=virtualdisk
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Capacity",type=string,JSONPath=`.status.capacity`
// +kubebuilder:printcolumn:name="Progress",type=string,JSONPath=`.status.progress`,priority=1
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.spec.persistentVolumeClaim.storageClassName`,priority=1
// +kubebuilder:printcolumn:name="TargetPVC",type=string,JSONPath=`.status.target.persistentVolumeClaimName`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 128",message="The name must be no longer than 128 characters."
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
	PersistentVolumeClaim VirtualDiskPersistentVolumeClaim `json:"persistentVolumeClaim,omitempty"`
}

type VirtualDiskStatus struct {
	DownloadSpeed *StatusSpeed `json:"downloadSpeed,omitempty"`
	// Requested capacity of the PVC in human-readable format.
	// +kubebuilder:example:="50G"
	Capacity string     `json:"capacity,omitempty"`
	Target   DiskTarget `json:"target,omitempty"`
	// Progress of copying an image from source to PVC. Appears only during the `Provisioning' phase.
	Progress string `json:"progress,omitempty"`
	// Deprecated: use ImageUploadURLs instead.
	UploadCommand   string           `json:"uploadCommand,omitempty"`
	ImageUploadURLs *ImageUploadURLs `json:"imageUploadURLs,omitempty"`
	Phase           DiskPhase        `json:"phase,omitempty"`
	// A list of `VirtualMachines` that use the disk
	// +kubebuilder:example:={{name: VM100}}
	AttachedToVirtualMachines []AttachedVirtualMachine `json:"attachedToVirtualMachines,omitempty"`
	Stats                     VirtualDiskStats         `json:"stats,omitempty"`
	SourceUID                 *types.UID               `json:"sourceUID,omitempty"`
	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// The generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// The name of the StorageClass used by the PersistentVolumeClaim if `Kubernetes` storage type used.
	StorageClassName string `json:"storageClassName,omitempty"`
}

// VirtualDisk statistics
type VirtualDiskStats struct {
	// The waiting time for the virtual disk creation.
	CreationDuration VirtualDiskStatsCreationDuration `json:"creationDuration,omitempty"`
}

type VirtualDiskStatsCreationDuration struct {
	// The waiting time for dependent resources.
	// +nullable
	WaitingForDependencies *metav1.Duration `json:"waitingForDependencies,omitempty"`
	// Duration of the loading into DVCR.
	// +nullable
	DVCRProvisioning *metav1.Duration `json:"dvcrProvisioning,omitempty"`
	// The duration of resource creation from the moment dependencies are ready until the resource transitions to the Ready state.
	// +nullable
	TotalProvisioning *metav1.Duration `json:"totalProvisioning,omitempty"`
}

// A list of `VirtualMachines` that use the disk
type AttachedVirtualMachine struct {
	Name string `json:"name,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'HTTP' ? has(self.http) && !has(self.containerImage) && !has(self.objectRef) : true",message="HTTP requires http and cannot have ContainerImage or ObjectRef"
// +kubebuilder:validation:XValidation:rule="self.type == 'ContainerImage' ? has(self.containerImage) && !has(self.http) && !has(self.objectRef) : true",message="ContainerImage requires containerImage and cannot have HTTP or ObjectRef"
// +kubebuilder:validation:XValidation:rule="self.type == 'ObjectRef' ? has(self.objectRef) && !has(self.http) && !has(self.containerImage) : true",message="ObjectRef requires objectRef and cannot have HTTP or ContainerImage"
type VirtualDiskDataSource struct {
	Type           DataSourceType             `json:"type,omitempty"`
	HTTP           *DataSourceHTTP            `json:"http,omitempty"`
	ContainerImage *VirtualDiskContainerImage `json:"containerImage,omitempty"`
	ObjectRef      *VirtualDiskObjectRef      `json:"objectRef,omitempty"`
}

// Use an image stored in external container registry. Only TLS enabled registries are supported. Use caBundle field to provide custom CA chain if needed.
type VirtualDiskContainerImage struct {
	// The container registry address of an image.
	// +kubebuilder:example:="registry.example.com/images/slackware:15"
	// +kubebuilder:validation:Pattern:=`^(?P<name>(?:(?P<domain>(?:(?:localhost|[\w-]+(?:\.[\w-]+)+)(?::\d+)?)|[\w]+:\d+)/)?(?P<image>[a-z0-9_.-]+(?:/[a-z0-9_.-]+)*))(?::(?P<tag>[\w][\w.-]{0,127}))?(?:@(?P<digest>[A-Za-z][A-Za-z0-9]*(?:[+.-_][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}))?$`
	Image           string              `json:"image"`
	ImagePullSecret ImagePullSecretName `json:"imagePullSecret,omitempty"`
	// The CA chain in base64 format to verify the container registry.
	// +kubebuilder:example:="YWFhCg=="
	CABundle []byte `json:"caBundle,omitempty"`
}

// Use an existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDiskSnapshot` to create a disk.
type VirtualDiskObjectRef struct {
	// A kind of existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDiskSnapshot`.
	Kind VirtualDiskObjectRefKind `json:"kind"`
	// A name of existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDiskSnapshot`.
	Name string `json:"name"`
}

// +kubebuilder:validation:Enum:={ClusterVirtualImage,VirtualImage,VirtualDiskSnapshot}
type VirtualDiskObjectRefKind string

const (
	VirtualDiskObjectRefKindVirtualImage        VirtualDiskObjectRefKind = "VirtualImage"
	VirtualDiskObjectRefKindClusterVirtualImage VirtualDiskObjectRefKind = "ClusterVirtualImage"
	VirtualDiskObjectRefKindVirtualDiskSnapshot VirtualDiskObjectRefKind = "VirtualDiskSnapshot"
)

type DiskTarget struct {
	// Created PersistentVolumeClaim name for Kubernetes storage.
	PersistentVolumeClaim string `json:"persistentVolumeClaimName,omitempty"`
}

// Settings for creating PVCs to store the disk.
type VirtualDiskPersistentVolumeClaim struct {
	// The name of the StorageClass required by the claim. More info — https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1
	//
	// When creating disks, the user can specify the required StorageClass to create the disk, or not explicitly, in which case the default StorageClass will be used.
	//
	// The disk features and virtual machine behavior depend on the selected StorageClass.
	//
	// The `VolumeBindingMode` parameter in the StorageClass affects the disk creation process:
	// - `Immediate` - The disk will be created and available for use immediately after creation.
	// - `WaitForFirstConsumer` - The disk will be created only when it is used in a virtual machine. In this case, the disk will be created on the host where the virtual machine will be started.
	//
	// StorageClass can support different storage settings:
	// - Creating a block device (`Block`) or file system (`FileSystem`).
	// - Multiple Access (`ReadWriteMany`) or Single Access (`ReadWriteOnce`). `ReadWriteMany` disks support multiple access, which enables live migration of virtual machines. In contrast, `ReadWriteOnce` disks, which are limited to access from only one host, cannot provide this capability.
	//
	// For known storage types, the platform will independently determine the most effective settings when creating disks (in descending order of priority):
	// 1. `Block` + `ReadWriteMany`
	// 2. `FileSystem` + `ReadWriteMany`
	// 3. `Block` + `ReadWriteOnce`
	// 4. `FileSystem` + `ReadWriteOnce`
	StorageClass *string `json:"storageClassName,omitempty"`
	// Desired size for PVC to store the disk. If the disk is created from an image, the size must be at least as large as the original unpacked image.
	//
	// This parameter can be omitted if the `.spec.dataSource` block is specified, in which case the controller will determine the disk size automatically, based on the size of the extracted image from the source specified in `.spec.dataSource`.
	Size *resource.Quantity `json:"size,omitempty"`
}

// VirtualDiskList contains a list of VirtualDisk
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualDiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualDisk `json:"items"`
}

// Current status of `VirtualDisk` resource:
// * Pending - The resource has been created and is on a waiting queue.
// * Provisioning - The process of resource creation (copying/downloading/filling the PVC with data/extending PVC) is in progress.
// * WaitForUserUpload - Waiting for the user to upload the image. The endpoint to upload the image is specified in `.status.uploadCommand`.
// * WaitForFirstConsumer - Waiting for the virtual machine that uses the disk is scheduled.
// * Ready - The resource is created and ready to use.
// * Resizing — The process of resource resizing is in progress.
// * Failed - There was a problem when creating a resource.
// * PVCLost - The child PVC of the resource is missing. The resource cannot be used.
// * Terminating - The process of resource deletion is in progress.
// +kubebuilder:validation:Enum:={Pending,Provisioning,WaitForUserUpload,Ready,Failed,Terminating,PVCLost,WaitForFirstConsumer,Resizing}
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
