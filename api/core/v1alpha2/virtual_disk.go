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

// The VirtualDisk resource describes the desired virtual machine disk configuration. A VirtualDisk can be mounted statically in the virtual machine by specifying it in the `.spec.blockDeviceRefs` disk list, or mounted on-the-fly using the VirtualMachineBlockDeviceAttachments resource.
//
// Once a VirtualDisk is created, only the disk size field `.spec.persistentVolumeClaim.size` can be changed. All other fields are immutable.
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization},scope=Namespaced,shortName={vd},singular=virtualdisk
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Capacity",type=string,JSONPath=`.status.capacity`
// +kubebuilder:printcolumn:name="InUse",type=string,JSONPath=`.status.conditions[?(@.type=='InUse')].status`,priority=1
// +kubebuilder:printcolumn:name="Progress",type=string,JSONPath=`.status.progress`,priority=1
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.status.storageClassName`,priority=1
// +kubebuilder:printcolumn:name="TargetPVC",type=string,JSONPath=`.status.target.persistentVolumeClaimName`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
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
	// Requested PVC capacity in human-readable format.
	// +kubebuilder:example:="50G"
	Capacity string     `json:"capacity,omitempty"`
	Target   DiskTarget `json:"target,omitempty"`
	// Progress of copying an image from a source to PVC. Appears only during the `Provisioning' phase.
	Progress string `json:"progress,omitempty"`
	// Deprecated. Use `ImageUploadURLs` instead.
	UploadCommand   string           `json:"uploadCommand,omitempty"`
	ImageUploadURLs *ImageUploadURLs `json:"imageUploadURLs,omitempty"`
	Phase           DiskPhase        `json:"phase,omitempty"`
	// List of VirtualMachines that use the disk.
	// +kubebuilder:example:={{name: VM100}}
	AttachedToVirtualMachines []AttachedVirtualMachine `json:"attachedToVirtualMachines,omitempty"`
	Stats                     VirtualDiskStats         `json:"stats,omitempty"`
	SourceUID                 *types.UID               `json:"sourceUID,omitempty"`
	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Name of the StorageClass used by the PersistentVolumeClaim if `Kubernetes` storage type is used.
	StorageClassName string `json:"storageClassName,omitempty"`

	// Migration information.
	MigrationState VirtualDiskMigrationState `json:"migrationState,omitempty"`
}

// VirtualDisk statistics.
type VirtualDiskStats struct {
	// Waiting time for the virtual disk creation.
	CreationDuration VirtualDiskStatsCreationDuration `json:"creationDuration,omitempty"`
}

type VirtualDiskStatsCreationDuration struct {
	// Waiting time for dependent resources.
	// +nullable
	WaitingForDependencies *metav1.Duration `json:"waitingForDependencies,omitempty"`
	// Duration of the loading into DVCR.
	// +nullable
	DVCRProvisioning *metav1.Duration `json:"dvcrProvisioning,omitempty"`
	// Duration of the resource creation from the moment dependencies are ready until the resource transitions to the Ready state.
	// +nullable
	TotalProvisioning *metav1.Duration `json:"totalProvisioning,omitempty"`
}

// List of VirtualMachines that use the disk.
type AttachedVirtualMachine struct {
	// Name of attached VirtualMachine.
	Name string `json:"name,omitempty"`
	// Flag indicating that VirtualDisk is currently being used by this attached VirtualMachine.
	Mounted bool `json:"mounted,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'HTTP' ? has(self.http) && !has(self.containerImage) && !has(self.objectRef) : true",message="HTTP requires http and cannot have ContainerImage or ObjectRef."
// +kubebuilder:validation:XValidation:rule="self.type == 'ContainerImage' ? has(self.containerImage) && !has(self.http) && !has(self.objectRef) : true",message="ContainerImage requires containerImage and cannot have HTTP or ObjectRef."
// +kubebuilder:validation:XValidation:rule="self.type == 'ObjectRef' ? has(self.objectRef) && !has(self.http) && !has(self.containerImage) : true",message="ObjectRef requires objectRef and cannot have HTTP or ContainerImage."
type VirtualDiskDataSource struct {
	Type           DataSourceType             `json:"type,omitempty"`
	HTTP           *DataSourceHTTP            `json:"http,omitempty"`
	ContainerImage *VirtualDiskContainerImage `json:"containerImage,omitempty"`
	ObjectRef      *VirtualDiskObjectRef      `json:"objectRef,omitempty"`
}

// Use an image stored in an external container registry. Only registries with enabled TLS are supported. To provide a custom Certificate Authority (CA) chain, use the `caBundle` field.
type VirtualDiskContainerImage struct {
	// Path to the image in the container registry.
	// +kubebuilder:example:="registry.example.com/images/slackware:15"
	// +kubebuilder:validation:Pattern:=`^(?P<name>(?:(?P<domain>(?:(?:localhost|[\w-]+(?:\.[\w-]+)+)(?::\d+)?)|[\w]+:\d+)/)?(?P<image>[a-z0-9_.-]+(?:/[a-z0-9_.-]+)*))(?::(?P<tag>[\w][\w.-]{0,127}))?(?:@(?P<digest>[A-Za-z][A-Za-z0-9]*(?:[+.-_][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}))?$`
	Image           string              `json:"image"`
	ImagePullSecret ImagePullSecretName `json:"imagePullSecret,omitempty"`
	// CA chain in Base64 format to verify the container registry.
	// +kubebuilder:example:="YWFhCg=="
	CABundle []byte `json:"caBundle,omitempty"`
}

// Use an existing VirtualImage, ClusterVirtualImage, or VirtualDiskSnapshot resource to create a disk.
type VirtualDiskObjectRef struct {
	// Kind of the existing VirtualImage, ClusterVirtualImage, or VirtualDiskSnapshot resource.
	Kind VirtualDiskObjectRefKind `json:"kind"`
	// Name of the existing VirtualImage, ClusterVirtualImage, or VirtualDiskSnapshot resource.
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
	// Created PersistentVolumeClaim name for the Kubernetes storage.
	PersistentVolumeClaim string `json:"persistentVolumeClaimName,omitempty"`
}

// Settings for creating PVCs to store the disk.
type VirtualDiskPersistentVolumeClaim struct {
	// StorageClass name required by the claim. For details on using StorageClass for PVC, refer to https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1.
	//
	// When creating disks, the user can specify the required StorageClass. If not specified, the default StorageClass will be used.
	//
	// The disk features and virtual machine behavior depend on the selected StorageClass.
	//
	// The `VolumeBindingMode` parameter in the StorageClass affects the disk creation process. The following values are allowed:
	// - `Immediate`: The disk will be created and becomes available for use immediately after creation.
	// - `WaitForFirstConsumer`: The disk will be created when first used on the node where the virtual machine will be started.
	//
	// StorageClass supports multiple storage settings:
	// - Creating a block device (`Block`) or file system (`FileSystem`).
	// - Multiple access (`ReadWriteMany`) or single access (`ReadWriteOnce`). The `ReadWriteMany` disks support multiple access, which enables a "live" migration of virtual machines. In contrast, the `ReadWriteOnce` disks, which can be accessed from only one node, don't have this feature.
	//
	// For known storage types, Deckhouse automatically determines the most efficient settings when creating disks (by priority, in descending order):
	// 1. `Block` + `ReadWriteMany`
	// 2. `FileSystem` + `ReadWriteMany`
	// 3. `Block` + `ReadWriteOnce`
	// 4. `FileSystem` + `ReadWriteOnce`
	StorageClass *string `json:"storageClassName,omitempty"`
	// Desired size for PVC to store the disk. If the disk is created from an image, the size must be at least as large as the original unpacked image.
	//
	// This parameter can be omitted if the `.spec.dataSource` section is filled out. In this case, the controller will determine the disk size automatically, based on the size of the extracted image from the source specified in `.spec.dataSource`.
	Size *resource.Quantity `json:"size,omitempty"`
}

// VirtualDiskList contains a list of VirtualDisks.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualDiskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualDisk `json:"items"`
}

// Current status of the VirtualDisk resource:
// * `Pending`: The resource has been created and is on a waiting queue.
// * `Provisioning`: The resource is being created: copying, downloading, loading data to the PVC, or extending the PVC.
// * `WaitForUserUpload`: Waiting for the user to upload the image. The endpoint to upload the image is specified in `.status.uploadCommand`.
// * `WaitForFirstConsumer`: Waiting for the virtual machine using the disk to be assigned to the node.
// * `Ready`: The resource has been created and is ready to use.
// * `Resizing`: The process of resource resizing is in progress.
// * `Failed`: There was an error when creating the resource.
// * `PVCLost`: The child PVC of the resource is missing. The resource cannot be used.
// * `Exporting`: The child PV of the resource is in the process of exporting.
// * `Terminating`: The resource is being deleted.
// * `Migrating`: The resource is being migrating.
// +kubebuilder:validation:Enum:={Pending,Provisioning,WaitForUserUpload,WaitForFirstConsumer,Ready,Resizing,Failed,PVCLost,Exporting,Terminating,Migrating}
type DiskPhase string

const (
	DiskPending              DiskPhase = "Pending"
	DiskProvisioning         DiskPhase = "Provisioning"
	DiskWaitForUserUpload    DiskPhase = "WaitForUserUpload"
	DiskWaitForFirstConsumer DiskPhase = "WaitForFirstConsumer"
	DiskReady                DiskPhase = "Ready"
	DiskResizing             DiskPhase = "Resizing"
	DiskFailed               DiskPhase = "Failed"
	DiskLost                 DiskPhase = "PVCLost"
	DiskExporting            DiskPhase = "Exporting"
	DiskTerminating          DiskPhase = "Terminating"
	DiskMigrating            DiskPhase = "Migrating"
)

type VirtualDiskMigrationState struct {
	// Source PersistentVolumeClaim name.
	SourcePVC string `json:"sourcePVC,omitempty"`
	// Target PersistentVolumeClaim name.
	TargetPVC      string                     `json:"targetPVC,omitempty"`
	Result         VirtualDiskMigrationResult `json:"result,omitempty"`
	Message        string                     `json:"message,omitempty"`
	StartTimestamp metav1.Time                `json:"startTimestamp,omitempty"`
	EndTimestamp   metav1.Time                `json:"endTimestamp,omitempty"`
}

// VirtualDiskMigrationResult is the result of the VirtualDisk migration.
// +kubebuilder:validation:Enum=Succeeded;Failed
type VirtualDiskMigrationResult string

const (
	VirtualDiskMigrationResultSucceeded VirtualDiskMigrationResult = "Succeeded"
	VirtualDiskMigrationResultFailed    VirtualDiskMigrationResult = "Failed"
)
