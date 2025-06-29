package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The VirtualDataExport describes the configuration and status of a virtual data export (VDExport).
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:categories={virtualization,all},scope=Namespaced,shortName={vde,vdes},singular=virtualdataexport
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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
	// +kubebuilder:default:="30m"
	// +kubebuilder:validation:Format=duration
	IdleTimeout metav1.Duration            `json:"idleTimeout,omitempty"`
	TargetRef   VirtualDataExportTargetRef `json:"targetRef"`
}

type VirtualDataExportTargetRef struct {
	Kind VirtualDataExportTargetKind `json:"kind"`
	Name string                      `json:"name"`
}

type VirtualDataExportTargetKind string

const (
	VirtualDataExportTargetVirtualDisk         VirtualDataExportTargetKind = "VirtualDisk"
	VirtualDataExportTargetVirtualDiskSnapshot VirtualDataExportTargetKind = "VirtualDiskSnapshot"
	VirtualDataExportTargetVirtualImage        VirtualDataExportTargetKind = "VirtualImage"
	VirtualDataExportTargetClusterVirtualImage VirtualDataExportTargetKind = "ClusterVirtualImage"
)

type VirtualDataExportStatus struct {
	URL                string             `json:"url,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}
