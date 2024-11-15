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
	ClusterVirtualImageKind     = "ClusterVirtualImage"
	ClusterVirtualImageResource = "clustervirtualimages"
)

// Describes a virtual disk image that can be used as a data source for new `VirtualDisks` or an installation image (iso) to be mounted in `Virtuals` directly. This resource type is available for all namespaces in the cluster.
//
// > This resource cannot be modified once it has been created.
//
// A container image is created under the hood of this resource, which is stored in a dedicated deckhouse virtualization container registry (DVCR).
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization,backup.deckhouse.io/cluster-config=true}
// +kubebuilder:resource:categories={virtualization},scope=Cluster,shortName={cvi,cvis},singular=clustervirtualimage
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="CDROM",type=boolean,JSONPath=`.status.cdrom`
// +kubebuilder:printcolumn:name="Progress",type=string,JSONPath=`.status.progress`
// +kubebuilder:printcolumn:name="StoredSize",type=string,JSONPath=`.status.size.stored`,priority=1
// +kubebuilder:printcolumn:name="UnpackedSize",type=string,JSONPath=`.status.size.unpacked`,priority=1
// +kubebuilder:printcolumn:name="Registry URL",type=string,JSONPath=`.status.target.registryURL`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="self.metadata.name.size() <= 128",message="The name must be no longer than 128 characters."
// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterVirtualImage struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterVirtualImageSpec `json:"spec"`

	Status ClusterVirtualImageStatus `json:"status,omitempty"`
}

// ClusterVirtualImageList provides the needed parameters
// to do request a list of ClusterVirtualImages from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterVirtualImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs
	Items []ClusterVirtualImage `json:"items"`
}

type ClusterVirtualImageSpec struct {
	DataSource ClusterVirtualImageDataSource `json:"dataSource"`
}

// An origin of the image.
// +kubebuilder:validation:XValidation:rule="self.type == 'HTTP' ? has(self.http) && !has(self.containerImage) && !has(self.objectRef) : true",message="HTTP requires http and cannot have ContainerImage or ObjectRef"
// +kubebuilder:validation:XValidation:rule="self.type == 'ContainerImage' ? has(self.containerImage) && !has(self.http) && !has(self.objectRef) : true",message="ContainerImage requires containerImage and cannot have HTTP or ObjectRef"
// +kubebuilder:validation:XValidation:rule="self.type == 'ObjectRef' ? has(self.objectRef) && !has(self.http) && !has(self.containerImage) : true",message="ObjectRef requires objectRef and cannot have HTTP or ContainerImage"
type ClusterVirtualImageDataSource struct {
	Type           DataSourceType                     `json:"type"`
	HTTP           *DataSourceHTTP                    `json:"http,omitempty"`
	ContainerImage *ClusterVirtualImageContainerImage `json:"containerImage,omitempty"`
	ObjectRef      *ClusterVirtualImageObjectRef      `json:"objectRef,omitempty"`
}

// Use an image stored in external container registry. Only TLS enabled registries are supported. Use caBundle field to provide custom CA chain if needed.
type ClusterVirtualImageContainerImage struct {
	// The container registry address of an image.
	// +kubebuilder:example:="registry.example.com/images/slackware:15"
	// +kubebuilder:validation:Pattern:=`^(?P<name>(?:(?P<domain>(?:(?:localhost|[\w-]+(?:\.[\w-]+)+)(?::\d+)?)|[\w]+:\d+)/)?(?P<image>[a-z0-9_.-]+(?:/[a-z0-9_.-]+)*))(?::(?P<tag>[\w][\w.-]{0,127}))?(?:@(?P<digest>[A-Za-z][A-Za-z0-9]*(?:[+.-_][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}))?$`
	Image           string          `json:"image"`
	ImagePullSecret ImagePullSecret `json:"imagePullSecret,omitempty"`
	// The CA chain in base64 format to verify the container registry.
	// +kubebuilder:example:="YWFhCg=="
	CABundle []byte `json:"caBundle,omitempty"`
}

// Use an existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDisk` to create an image.
//
// +kubebuilder:validation:XValidation:rule="self.kind == 'VirtualImage' || self.kind == 'VirtualDisk' ? has(self.__namespace__) && size(self.__namespace__) > 0 : true",message="The namespace is required for VirtualDisk and VirtualImage"
// +kubebuilder:validation:XValidation:rule="self.kind == 'VirtualImage' || self.kind == 'VirtualDisk' ? has(self.__namespace__) && size(self.__namespace__) < 64 : true",message="The namespace must be no longer than 63 characters."
type ClusterVirtualImageObjectRef struct {
	Kind ClusterVirtualImageObjectRefKind `json:"kind"`
	// A name of existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDisk`.
	Name string `json:"name"`
	// A namespace where `VirtualImage` or `VirtualDisk` is located.
	Namespace string `json:"namespace,omitempty"`
}

// A kind of existing `VirtualImage`, `ClusterVirtualImage` or `VirtualDisk`.
// +kubebuilder:validation:Enum:={ClusterVirtualImage,VirtualImage,VirtualDisk}
type ClusterVirtualImageObjectRefKind string

const (
	ClusterVirtualImageObjectRefKindVirtualImage        ClusterVirtualImageObjectRefKind = "VirtualImage"
	ClusterVirtualImageObjectRefKindClusterVirtualImage ClusterVirtualImageObjectRefKind = "ClusterVirtualImage"
	ClusterVirtualImageObjectRefKindVirtualDisk         ClusterVirtualImageObjectRefKind = "VirtualDisk"
)

type ClusterVirtualImageStatus struct {
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
	// +kubebuilder:validation:Enum:={Pending,Provisioning,WaitForUserUpload,Ready,Failed,Terminating}
	Phase ImagePhase `json:"phase,omitempty"`
	// Progress of copying an image from source to DVCR. Appears only during the `Provisioning' phase.
	Progress string `json:"progress,omitempty"`
	// The UID of the source (`VirtualImage`, `ClusterVirtualImage` or `VirtualDisk`) used when creating the cluster virtual image.
	SourceUID *types.UID `json:"sourceUID,omitempty"`
	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// The generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Deprecated. Use imageUploadURLs instead.
	UploadCommand   string                          `json:"uploadCommand,omitempty"`
	ImageUploadURLs *ImageUploadURLs                `json:"imageUploadURLs,omitempty"`
	Target          ClusterVirtualImageStatusTarget `json:"target,omitempty"`
}

type ClusterVirtualImageStatusTarget struct {
	// Created image in DVCR.
	// +kubebuilder:example:="dvcr.<dvcr-namespace>.svc/cvi/<image-name>:latest"
	RegistryURL string `json:"registryURL,omitempty"`
}
