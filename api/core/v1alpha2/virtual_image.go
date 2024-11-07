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
	Storage StorageType `json:"storage"`
	// PersistentVolumeClaim VirtualImagePersistentVolumeClaim `json:"persistentVolumeClaim,omitempty"`

	DataSource VirtualImageDataSource `json:"dataSource"`
}

type VirtualImageStatus struct {
	ImageStatus        `json:",inline"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	StorageClassName   string             `json:"storageClassName,omitempty"`
}

type VirtualImageStatusTarget struct {
	// Created image in DVCR.
	// +kubebuilder:example:="dvcr.<dvcr-namespace>.svc/vi/<image-namespace>/<image-name>:latest"
	RegistryURL string `json:"registryURL,omitempty"`
	// FIXME: create ClusterImageStatus without Capacity and PersistentVolumeClaim

	// Created PersistentVolumeClaim name for Kubernetes storage.
	PersistentVolumeClaim string `json:"persistentVolumeClaimName,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.type == 'HTTP' ? has(self.http) && !has(self.containerImage) && !has(self.objectRef) : true",message="HTTP requires http and cannot have ContainerImage or ObjectRef"
// +kubebuilder:validation:XValidation:rule="self.type == 'ContainerImage' ? has(self.containerImage) && !has(self.http) && !has(self.objectRef) : true",message="ContainerImage requires containerImage and cannot have HTTP or ObjectRef"
// +kubebuilder:validation:XValidation:rule="self.type == 'ObjectRef' ? has(self.objectRef) && !has(self.http) && !has(self.containerImage) : true",message="ObjectRef requires objectRef and cannot have HTTP or ContainerImage"
type VirtualImageDataSource struct {
	Type           DataSourceType               `json:"type,omitempty"`
	HTTP           *DataSourceHTTP              `json:"http,omitempty"`
	ContainerImage *DataSourceContainerRegistry `json:"containerImage,omitempty"`
	ObjectRef      *VirtualImageObjectRef       `json:"objectRef,omitempty"`
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
// * `Kubernetes` - use a Persistent Volume Claim (PVC).
// +kubebuilder:validation:Enum:={ContainerRegistry,Kubernetes}
type StorageType string

const (
	StorageContainerRegistry StorageType = "ContainerRegistry"
	StorageKubernetes        StorageType = "Kubernetes"
)

type VirtualImagePersistentVolumeClaim struct {
	StorageClass *string `json:"storageClass,omitempty"`
}
