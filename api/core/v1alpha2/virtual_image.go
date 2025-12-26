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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	VirtualImageKind     = "VirtualImage"
	VirtualImageResource = "virtualimages"
)

// This resource describes a virtual disk image to use as a data source for new VirtualDisk resources or an installation image (iso) that can be mounted into the VirtualMachine resource.
//
// > This resource cannot be modified once it has been created.
//
// With this resource in the cluster, a container image is created and stored in a dedicated Deckhouse Virtualization Container Registry (DVCR) or PVC, with the data filled in from the source.
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization},scope=Namespaced,shortName={vi},singular=virtualimage
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="CDROM",type=boolean,JSONPath=`.status.cdrom`
// +kubebuilder:printcolumn:name="Progress",type=string,JSONPath=`.status.progress`
// +kubebuilder:printcolumn:name="StoredSize",type=string,JSONPath=`.status.size.stored`,priority=1
// +kubebuilder:printcolumn:name="UnpackedSize",type=string,JSONPath=`.status.size.unpacked`,priority=1
// +kubebuilder:printcolumn:name="Registry URL",type=string,JSONPath=`.status.target.registryURL`,priority=1
// +kubebuilder:printcolumn:name="TargetPVC",type=string,JSONPath=`.status.target.persistentVolumeClaimName`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualImage struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualImageSpec `json:"spec"`

	Status VirtualImageStatus `json:"status,omitempty"`
}

// VirtualImageList provides the needed parameters
// for requesting a list of VirtualImages from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs.
	Items []VirtualImage `json:"items"`
}

type VirtualImageSpec struct {
	// +kubebuilder:default:=ContainerRegistry
	Storage               StorageType                       `json:"storage"`
	PersistentVolumeClaim VirtualImagePersistentVolumeClaim `json:"persistentVolumeClaim,omitempty"`
	DataSource            VirtualImageDataSource            `json:"dataSource"`
}

type VirtualImageStatus struct {
	// Image download speed from an external source. Appears only during the `Provisioning` phase.
	DownloadSpeed *StatusSpeed `json:"downloadSpeed,omitempty"`
	// Discovered image size data.
	Size ImageStatusSize `json:"size,omitempty"`
	// Discovered image format.
	Format string `json:"format,omitempty"`
	// Whether the image is in a format that needs to be mounted as a CD-ROM drive, such as iso and so on.
	CDROM bool `json:"cdrom,omitempty"`
	// Current status of the ClusterVirtualImage resource:
	// * `Pending`: The resource has been created and is on a waiting queue.
	// * `Provisioning`: The resource is being created: copying, downloading, or building the image.
	// * `WaitForUserUpload`: Waiting for the user to upload the image. The endpoint to upload the image is specified in `.status.uploadCommand`.
	// * `Ready`: The resource has been created and is ready to use.
	// * `Failed`: There was an error when creating the resource.
	// * `Terminating`: The resource is being deleted.
	// * `ImageLost`: The image is missing in DVCR. The resource cannot be used.
	// * `PVCLost`: The child PVC of the resource is missing. The resource cannot be used.
	// +kubebuilder:validation:Enum:={Pending,Provisioning,WaitForUserUpload,Ready,Failed,Terminating,ImageLost,PVCLost}
	Phase ImagePhase `json:"phase,omitempty"`
	// Progress of copying an image from a source to DVCR.
	Progress string `json:"progress,omitempty"`
	// Deprecated. Use `imageUploadURLs` instead.
	UploadCommand   string                   `json:"uploadCommand,omitempty"`
	ImageUploadURLs *ImageUploadURLs         `json:"imageUploadURLs,omitempty"`
	Target          VirtualImageStatusTarget `json:"target,omitempty"`
	// UID of the source (VirtualImage, ClusterVirtualImage, VirtualDisk or VirtualDiskSnapshot) used when creating the virtual image.
	SourceUID *types.UID `json:"sourceUID,omitempty"`
	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Name of the StorageClass used by the PersistentVolumeClaim if `Kubernetes` storage type is used.
	StorageClassName string `json:"storageClassName,omitempty"`
}

type VirtualImageStatusTarget struct {
	// Created image in DVCR.
	// +kubebuilder:example:="dvcr.<dvcr-namespace>.svc/vi/<image-namespace>/<image-name>:latest"
	RegistryURL string `json:"registryURL,omitempty"`
	// Created PersistentVolumeClaim name for the PersistentVolumeClaim storage.
	PersistentVolumeClaim string `json:"persistentVolumeClaimName,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'HTTP' ? has(self.http) && !has(self.containerImage) && !has(self.objectRef) : true",message="HTTP requires http and cannot have ContainerImage or ObjectRef."
// +kubebuilder:validation:XValidation:rule="self.type == 'ContainerImage' ? has(self.containerImage) && !has(self.http) && !has(self.objectRef) : true",message="ContainerImage requires containerImage and cannot have HTTP or ObjectRef."
// +kubebuilder:validation:XValidation:rule="self.type == 'ObjectRef' ? has(self.objectRef) && !has(self.http) && !has(self.containerImage) : true",message="ObjectRef requires objectRef and cannot have HTTP or ContainerImage."
type VirtualImageDataSource struct {
	Type           DataSourceType              `json:"type,omitempty"`
	HTTP           *DataSourceHTTP             `json:"http,omitempty"`
	ContainerImage *VirtualImageContainerImage `json:"containerImage,omitempty"`
	ObjectRef      *VirtualImageObjectRef      `json:"objectRef,omitempty"`
}

// Use an image stored in an external container registry. Only registries with enabled TLS protocol are supported. To provide a custom Certificate Authority (CA) chain, use the `caBundle` field.
type VirtualImageContainerImage struct {
	// Path to the image in the container registry.
	// +kubebuilder:example:="registry.example.com/images/slackware:15"
	// +kubebuilder:validation:Pattern:=`^(?P<name>(?:(?P<domain>(?:(?:localhost|[\w-]+(?:\.[\w-]+)+)(?::\d+)?)|[\w]+:\d+)/)?(?P<image>[a-z0-9_.-]+(?:/[a-z0-9_.-]+)*))(?::(?P<tag>[\w][\w.-]{0,127}))?(?:@(?P<digest>[A-Za-z][A-Za-z0-9]*(?:[+.-_][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}))?$`
	Image           string              `json:"image"`
	ImagePullSecret ImagePullSecretName `json:"imagePullSecret,omitempty"`
	// CA chain in Base64 format to verify the container registry.
	// +kubebuilder:example:="YWFhCg=="
	CABundle []byte `json:"caBundle,omitempty"`
}

// Use an existing VirtualImage, ClusterVirtualImage, VirtualDisk or VirtualDiskSnapshot resource to create an image.
type VirtualImageObjectRef struct {
	// Kind of an existing VirtualImage, ClusterVirtualImage, VirtualDisk or VirtualDiskSnapshot resource.
	Kind VirtualImageObjectRefKind `json:"kind"`
	// Name of an existing VirtualImage, ClusterVirtualImage, VirtualDisk or VirtualDiskSnapshot resource.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// +kubebuilder:validation:Enum:={ClusterVirtualImage,VirtualImage,VirtualDisk,VirtualDiskSnapshot}
type VirtualImageObjectRefKind string

const (
	VirtualImageObjectRefKindVirtualImage        VirtualImageObjectRefKind = "VirtualImage"
	VirtualImageObjectRefKindClusterVirtualImage VirtualImageObjectRefKind = "ClusterVirtualImage"
	VirtualImageObjectRefKindVirtualDisk         VirtualImageObjectRefKind = "VirtualDisk"
	VirtualImageObjectRefKindVirtualDiskSnapshot VirtualImageObjectRefKind = "VirtualDiskSnapshot"
)

// Storage type to keep the image for the current virtualization setup.
//
// * `ContainerRegistry`: Use the DVCR container registry. In this case, images are downloaded to a container and then to DVCR (shipped with the virtualization module).
// * `PersistentVolumeClaim`: Use a PVC.
// * `Kubernetes`: A deprecated storage type. Not recommended for use and may be removed in future versions. Use `PersistentVolumeClaim` instead.
// +kubebuilder:validation:Enum:={ContainerRegistry,Kubernetes,PersistentVolumeClaim}
type StorageType string

const (
	StorageContainerRegistry     StorageType = "ContainerRegistry"
	StoragePersistentVolumeClaim StorageType = "PersistentVolumeClaim"

	// TODO: remove storage type Kubernetes in 2025
	StorageKubernetes StorageType = "Kubernetes"
)

// Settings for creating PVCs to store an image with the storage type `PersistentVolumeClaim`.
type VirtualImagePersistentVolumeClaim struct {
	// Name of the StorageClass required by the claim. For details on using StorageClass for PVC, refer to — https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1.
	//
	// When creating an image with the `PersistentVolumeClaim` storage type, the user can specify the required StorageClass. If not specified, the default StorageClass will be used.
	StorageClass *string `json:"storageClassName,omitempty"`
}
