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
	ClusterVirtualImageKind     = "ClusterVirtualImage"
	ClusterVirtualImageResource = "clustervirtualimages"
)

// +genclient:nonNamespaced

// ClusterVirtualImage is a cluster wide available image for virtual machines.
// +genclient
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

type ClusterVirtualImageDataSource struct {
	Type           DataSourceType                `json:"type,omitempty"`
	HTTP           *DataSourceHTTP               `json:"http,omitempty"`
	ContainerImage *DataSourceContainerRegistry  `json:"containerImage,omitempty"`
	ObjectRef      *ClusterVirtualImageObjectRef `json:"objectRef,omitempty"`
}

type ClusterVirtualImageObjectRef struct {
	Kind      ClusterVirtualImageObjectRefKind `json:"kind,omitempty"`
	Name      string                           `json:"name,omitempty"`
	Namespace string                           `json:"namespace,omitempty"`
}

type ClusterVirtualImageObjectRefKind string

const (
	ClusterVirtualImageObjectRefKindVirtualImage        ClusterVirtualImageObjectRefKind = "VirtualImage"
	ClusterVirtualImageObjectRefKindClusterVirtualImage ClusterVirtualImageObjectRefKind = "ClusterVirtualImage"
	ClusterVirtualImageObjectRefKindVirtualDisk         ClusterVirtualImageObjectRefKind = "VirtualDisk"
)

type ClusterVirtualImageStatus struct {
	ImageStatus        `json:",inline"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}
