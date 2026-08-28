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

	// The following fields carry state-snapshotter SDK bookkeeping (normative SnapshotAdapter contract,
	// see github.com/deckhouse/state-snapshotter/pkg/snapshotsdk). Populated only when the resource is
	// annotated with AnnUseUnifiedSnapshotter and driven by the unified-snapshotter SDK controller;
	// The previous custom Secret-based snapshot controller neither reads nor writes it.

	// BoundSnapshotContentName is the name of the cluster-scoped SnapshotContent bound to this node.
	// Written by the state-snapshotter core binder; read-only from the domain controller's perspective.
	BoundSnapshotContentName string `json:"boundSnapshotContentName,omitempty"`
	// SourceRef is the full identity of the live source object this node captured (Capture mode only),
	// published via the SDK's PublishSnapshotSource for later use by import/restore tooling.
	SourceRef *UnifiedSnapshotterSourceRef `json:"sourceRef,omitempty"`
	// CaptureState is the SDK capture state machine for this node (barrier 1/2 phase, capture request
	// names, core-owned success latches).
	CaptureState *UnifiedSnapshotterCaptureState `json:"captureState,omitempty"`
	// ChildrenSnapshotRefs is always empty on this data-bearing leaf; kept for uniformity across
	// snapshot kinds (mirrors state-snapshotter's own reference domain implementation).
	ChildrenSnapshotRefs []UnifiedSnapshotterChildRef `json:"childrenSnapshotRefs,omitempty"`
	// ExcludedRefs is the top-level mirror of the bound SnapshotContent's durable excludedRefs aggregate.
	// Written only by the core; a data-bearing leaf has no children, so this is normally empty.
	ExcludedRefs []UnifiedSnapshotterChildRef `json:"excludedRefs,omitempty"`

	// Data is the captured volume's durable data binding, mirrored verbatim from the bound SnapshotContent
	// by the state-snapshotter core once the data leg is captured. VirtualDisk provisioning restores from
	// Data.ArtifactRef (via a storage-foundation VolumeRestoreRequest) instead of cloning the plain
	// namespaced CSI VolumeSnapshot referenced by VolumeSnapshotName: the SDK-captured artifact is a
	// VolumeSnapshotContent that external-snapshotter refuses to bind a new VolumeSnapshot to.
	Data *UnifiedSnapshotterDataBinding `json:"data,omitempty"`
	// StorageClassName mirrors the captured disk's storage class at capture time. Used as the default
	// target storage class when cloning a new VirtualDisk from this snapshot and the caller doesn't
	// specify one.
	StorageClassName string `json:"storageClassName,omitempty"`
	// PersistentVolumeClaimSize mirrors the captured disk's requested PVC size (a resource.Quantity
	// string) at capture time. Used as the default target size when cloning a new VirtualDisk from this
	// snapshot and the caller doesn't specify one.
	PersistentVolumeClaimSize string `json:"persistentVolumeClaimSize,omitempty"`
}

type VirtualDiskSnapshotPhase string

const (
	VirtualDiskSnapshotPhasePending     VirtualDiskSnapshotPhase = "Pending"
	VirtualDiskSnapshotPhaseInProgress  VirtualDiskSnapshotPhase = "InProgress"
	VirtualDiskSnapshotPhaseReady       VirtualDiskSnapshotPhase = "Ready"
	VirtualDiskSnapshotPhaseFailed      VirtualDiskSnapshotPhase = "Failed"
	VirtualDiskSnapshotPhaseTerminating VirtualDiskSnapshotPhase = "Terminating"
)
