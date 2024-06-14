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
)

const (
	VirtualImageKind     = "VirtualImage"
	VirtualImageResource = "virtualimages"
)

// VirtualImage is an image for virtual machines available in the particular namespace.
// +genclient
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
	Storage               StorageType                       `json:"storage"`
	PersistentVolumeClaim VirtualImagePersistentVolumeClaim `json:"persistentVolumeClaim"`
	DataSource            VirtualImageDataSource            `json:"dataSource"`
}

type VirtualImageStatus struct {
	ImageStatus `json:",inline"`
}

type VirtualImageDataSource struct {
	Type           DataSourceType               `json:"type,omitempty"`
	HTTP           *DataSourceHTTP              `json:"http,omitempty"`
	ContainerImage *DataSourceContainerRegistry `json:"containerImage,omitempty"`
	ObjectRef      *VirtualImageObjectRef       `json:"objectRef,omitempty"`
}

type VirtualImageObjectRef struct {
	Kind VirtualImageObjectRefKind `json:"kind,omitempty"`
	Name string                    `json:"name,omitempty"`
}

type VirtualImageObjectRefKind string

const (
	VirtualImageObjectRefKindVirtualImage        VirtualImageObjectRefKind = "VirtualImage"
	VirtualImageObjectRefKindClusterVirtualImage VirtualImageObjectRefKind = "ClusterVirtualImage"
)

type StorageType string

const (
	StorageContainerRegistry StorageType = "ContainerRegistry"
	StorageKubernetes        StorageType = "Kubernetes"
)

type VirtualImagePersistentVolumeClaim struct {
	StorageClass *string `json:"storageClass,omitempty"`
}
