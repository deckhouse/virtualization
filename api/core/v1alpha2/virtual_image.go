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

// This resource describes a virtual disk image or installation image (iso) that can be used as a data source for new `VirtualDisks` or can be mounted in `Virtuals`.
// > This resource cannot be modified once it has been created.
//
// A container image is created under the hood of this resource, which is stored in a dedicated deckhouse virtualization container registy (DVCR) or PVC, into which the data from the source is filled.
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization,all},scope=Namespaced,shortName={vi,vis},singular=virtualimage
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="CDROM",type=boolean,JSONPath=`.status.cdrom`
// +kubebuilder:printcolumn:name="Progress",type=string,JSONPath=`.status.progress`
// +kubebuilder:printcolumn:name="StoredSize",type=string,JSONPath=`.status.size.stored`,priority=1
// +kubebuilder:printcolumn:name="UnpackedSize",type=string,JSONPath=`.status.size.unpacked`,priority=1
// +kubebuilder:printcolumn:name="Registry URL",type=string,JSONPath=`.status.target.registryURL`,priority=1
// +kubebuilder:printcolumn:name="TargetPVC",type=string,JSONPath=`.status.target.persistentVolumeClaimName`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 128",message="The name must be no longer than 128 characters."
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualImage struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualImageSpec `json:"spec"`

	Status VirtualImageStatus `json:"status,omitempty"`
}

// VirtualImageList provides the needed parameters
// to do request a list of VirtualImages from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs
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
	// Discovered sizes of the image.
	Size ImageStatusSize `json:"size,omitempty"`
	// Discovered format of the image.
	Format string `json:"format,omitempty"`
	// Whether the image is a format that is supposed to be mounted as a cdrom, such as iso and so on.
	CDROM bool `json:"cdrom,omitempty"`
	// Current status of `ClusterVirtualImage` resource:
	// * Pending - The resource has been created and is on a waiting queue.
	// * Provisioning - The process of resource creation (copying/downloading/building the image) is in progress.
	// * WaitForUserUpload - Waiting for the user to upload the image. The endpoint to upload the image is specified in `.status.uploadCommand`.
	// * Ready - The resource is created and ready to use.
	// * Failed - There was a problem when creating a resource.
	// * Terminating - The process of resource deletion is in progress.
	// * PVCLost - The child PVC of the resource is missing. The resource cannot be used.
	// +kubebuilder:validation:Enum:={Pending,Provisioning,WaitForUserUpload,Ready,Failed,Terminating,PVCLost}
	Phase ImagePhase `json:"phase,omitempty"`
	// Progress of copying an image from source to DVCR.
	Progress string `json:"progress,omitempty"`
	// Deprecated. Use imageUploadURLs instead.
	UploadCommand   string                   `json:"uploadCommand,omitempty"`
	ImageUploadURLs *ImageUploadURLs         `json:"imageUploadURLs,omitempty"`
	Target          VirtualImageStatusTarget `json:"target,omitempty"`
	// The UID of the source (`VirtualImage`, `ClusterVirtualImage` or `VirtualDisk`) used when creating the virtual image.
	SourceUID *types.UID `json:"sourceUID,omitempty"`
	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// The generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// The name of the StorageClass used by the PersistentVolumeClaim if `Kubernetes` storage type used.
	StorageClassName string `json:"storageClassName,omitempty"`
}

type VirtualImageStatusTarget struct {
	// Created image in DVCR.
	// +kubebuilder:example:="dvcr.<dvcr-namespace>.svc/vi/<image-namespace>/<image-name>:latest"
	RegistryURL string `json:"registryURL,omitempty"`
	// Created PersistentVolumeClaim name for PersistentVolumeClaim storage.
	PersistentVolumeClaim string `json:"persistentVolumeClaimName,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'HTTP' ? has(self.http) && !has(self.containerImage) && !has(self.objectRef) : true",message="HTTP requires http and cannot have ContainerImage or ObjectRef"
// +kubebuilder:validation:XValidation:rule="self.type == 'ContainerImage' ? has(self.containerImage) && !has(self.http) && !has(self.objectRef) : true",message="ContainerImage requires containerImage and cannot have HTTP or ObjectRef"
// +kubebuilder:validation:XValidation:rule="self.type == 'ObjectRef' ? has(self.objectRef) && !has(self.http) && !has(self.containerImage) : true",message="ObjectRef requires objectRef and cannot have HTTP or ContainerImage"
type VirtualImageDataSource struct {
	Type           DataSourceType              `json:"type,omitempty"`
	HTTP           *DataSourceHTTP             `json:"http,omitempty"`
	ContainerImage *VirtualImageContainerImage `json:"containerImage,omitempty"`
	ObjectRef      *VirtualImageObjectRef      `json:"objectRef,omitempty"`
}

// Use an image stored in external container registry. Only TLS enabled registries are supported. Use caBundle field to provide custom CA chain if needed.
type VirtualImageContainerImage struct {
	// The container registry address of an image.
	// +kubebuilder:example:="registry.example.com/images/slackware:15"
	// +kubebuilder:validation:Pattern:=`^(?P<name>(?:(?P<domain>(?:(?:localhost|[\w-]+(?:\.[\w-]+)+)(?::\d+)?)|[\w]+:\d+)/)?(?P<image>[a-z0-9_.-]+(?:/[a-z0-9_.-]+)*))(?::(?P<tag>[\w][\w.-]{0,127}))?(?:@(?P<digest>[A-Za-z][A-Za-z0-9]*(?:[+.-_][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}))?$`
	Image           string              `json:"image"`
	ImagePullSecret ImagePullSecretName `json:"imagePullSecret,omitempty"`
	// The CA chain in base64 format to verify the container registry.
	// +kubebuilder:example:="YWFhCg=="
	CABundle []byte `json:"caBundle,omitempty"`
}

// Use an existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDisk` to create an image.
type VirtualImageObjectRef struct {
	// A kind of existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDisk`.
	Kind VirtualImageObjectRefKind `json:"kind"`
	// A name of existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDisk`.
	Name string `json:"name"`
}

// +kubebuilder:validation:Enum:={ClusterVirtualImage,VirtualImage,VirtualDisk}
type VirtualImageObjectRefKind string

const (
	VirtualImageObjectRefKindVirtualImage        VirtualImageObjectRefKind = "VirtualImage"
	VirtualImageObjectRefKindClusterVirtualImage VirtualImageObjectRefKind = "ClusterVirtualImage"
	VirtualImageObjectRefKindVirtualDisk         VirtualImageObjectRefKind = "VirtualDisk"
)

// Storage type to store the image for current virtualization setup.
//
// * `ContainerRegistry` — use a dedicated deckhouse virtualization container registry (DVCR). In this case, images will be downloaded and injected to a container, then pushed to a DVCR (shipped with the virtualization module).
// * `PersistentVolumeClaim` - use a Persistent Volume Claim (PVC).
// * `Kubernetes` - Deprecated: Use of this value is discouraged and may be removed in future versions. Use PersistentVolumeClaim instead.
// +kubebuilder:validation:Enum:={ContainerRegistry,Kubernetes,PersistentVolumeClaim}
type StorageType string

const (
	StorageContainerRegistry     StorageType = "ContainerRegistry"
	StoragePersistentVolumeClaim StorageType = "PersistentVolumeClaim"

	// TODO: remove storage type Kubernetes in 2025
	StorageKubernetes StorageType = "Kubernetes"
)

// Settings for creating PVCs to store the image with storage type 'PersistentVolumeClaim'.
type VirtualImagePersistentVolumeClaim struct {
	// The name of the StorageClass required by the claim. More info — https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1
	//
	// When creating image with storage type 'PersistentVolumeClaim', the user can specify the required StorageClass to create the image, or not explicitly, in which case the default StorageClass will be used.
	StorageClass *string `json:"storageClassName,omitempty"`
}
