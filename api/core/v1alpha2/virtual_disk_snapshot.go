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
	VirtualDiskSnapshotKind     = "VirtualDiskSnapshot"
	VirtualDiskSnapshotResource = "virtualdisksnapshots"
)

// VirtualDiskSnapshot
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualDiskSnapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualDiskSnapshotSpec   `json:"spec"`
	Status VirtualDiskSnapshotStatus `json:"status,omitempty"`
}

// VirtualDiskSnapshotList contains a list of VirtualDiskSnapshot
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualDiskSnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VirtualDiskSnapshot `json:"items"`
}

type VirtualDiskSnapshotSpec struct {
	VirtualDiskName     string `json:"virtualDiskName"`
	RequiredConsistency bool   `json:"requiredConsistency"`
}

type VirtualDiskSnapshotStatus struct {
	Phase              VirtualDiskSnapshotPhase `json:"phase"`
	VolumeSnapshotName string                   `json:"volumeSnapshotName,omitempty"`
	Consistent         *bool                    `json:"consistent,omitempty"`
	Conditions         []metav1.Condition       `json:"conditions,omitempty"`
	ObservedGeneration int64                    `json:"observedGeneration,omitempty"`
}

type VirtualDiskSnapshotPhase string

const (
	VirtualDiskSnapshotPhasePending     VirtualDiskSnapshotPhase = "Pending"
	VirtualDiskSnapshotPhaseInProgress  VirtualDiskSnapshotPhase = "InProgress"
	VirtualDiskSnapshotPhaseReady       VirtualDiskSnapshotPhase = "Ready"
	VirtualDiskSnapshotPhaseFailed      VirtualDiskSnapshotPhase = "Failed"
	VirtualDiskSnapshotPhaseTerminating VirtualDiskSnapshotPhase = "Terminating"
)
