/*
Copyright 2025 Flant JSC

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

const VirtualDataExportKind = "VirtualDataExport"

// The VirtualDataExport describes the configuration and status of a virtual data export (VDExport).
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization,all},scope=Namespaced,shortName={vde,vdes},singular=virtualdataexport
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Completed",type="string",JSONPath=".status.conditions[?(@.type=='Completed')].reason",description="VirtualDataExport completion status."
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualDataExport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualDataExportSpec   `json:"spec"`
	Status VirtualDataExportStatus `json:"status,omitempty"`
}

// The VirtualDataExportList provides the needed parameters
// for requesting a list of VirtualDataExports from the system.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualDataExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items provides a list of CDIs.
	Items []VirtualDataExport `json:"items"`
}

type VirtualDataExportSpec struct {
	// IdleTimeout defines the maximum duration to wait for a user to initiate the download.
	// The export is considered **Expired** if:
	//   1. The user never initiated the download, **AND**
	//   2. The specified (or default) `.spec.idleTimeout` period has elapsed.
	// Default: "30m" (30 minutes).
	// Must be a valid duration string (e.g., "1h", "30m", "60s").
	// +kubebuilder:default:="30m"
	// +kubebuilder:validation:Format=duration
	IdleTimeout metav1.Duration `json:"idleTimeout,omitempty"`

	// TargetRef defines the destination where data will be exported.
	// Contains reference information about the export target.
	TargetRef VirtualDataExportTargetRef `json:"targetRef"`
}

type VirtualDataExportTargetRef struct {
	Kind VirtualDataExportTargetKind `json:"kind"`
	// Name identifies the specific instance of the target resource.
	// This should match the metadata.name of the referenced resource.
	Name string `json:"name"`
}

// VirtualDataExportTargetKind defines the supported types of export targets.
// +kubebuilder:validation:Enum={VirtualDisk,VirtualDiskSnapshot,VirtualImage,ClusterVirtualImage}
type VirtualDataExportTargetKind string

const (
	VirtualDataExportTargetVirtualDisk         VirtualDataExportTargetKind = "VirtualDisk"
	VirtualDataExportTargetVirtualDiskSnapshot VirtualDataExportTargetKind = "VirtualDiskSnapshot"
	VirtualDataExportTargetVirtualImage        VirtualDataExportTargetKind = "VirtualImage"
	VirtualDataExportTargetClusterVirtualImage VirtualDataExportTargetKind = "ClusterVirtualImage"
)

// VirtualDataExportStatus defines the observed state of a VirtualDataExport resource.
type VirtualDataExportStatus struct {
	// The URL represents the download endpoint where the exported data can be accessed.
	// This field is populated when the export is ready for download.
	// The URL is typically valid until the export expires or is deleted.
	URL string `json:"url,omitempty"`

	// The latest available observations of an object's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
