/*
Copyright 2026 Flant JSC

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

// AnnUseUnifiedSnapshotter, when present on a VirtualMachineSnapshot or VirtualDiskSnapshot, routes that
// object to the unified-snapshotter SDK-based controller instead of the custom Secret-based one. See
// UnifiedSnapshotterCaptureState.
const AnnUseUnifiedSnapshotter = "virtualization.deckhouse.io/use-unified-snapshotter"

// UnifiedSnapshotterConditionReady is the condition type the unified-snapshotter SDK controller writes
// into the existing status.conditions of an annotated VirtualMachineSnapshot/VirtualDiskSnapshot,
// mirroring the bound SnapshotContent's readiness. It is the single user-facing condition; the core
// (external state-snapshotter module) is its sole writer.
const UnifiedSnapshotterConditionReady = "Ready"

// UnifiedSnapshotterPhase mirrors github.com/deckhouse/state-snapshotter's
// storage/v1alpha1.SnapshotCapturePhase (status.captureState.domainSpecificController.phase): the
// domain-owned capture lifecycle. Monotonic: Planning -> Planned (children, data and manifest legs
// declared) -> Finished (domain consistency actions done). Failed is a terminal sink outside this chain.
//
// +kubebuilder:validation:Enum={Planning,Planned,Finished,Failed}
type UnifiedSnapshotterPhase string

const (
	UnifiedSnapshotterPhasePlanning UnifiedSnapshotterPhase = "Planning"
	UnifiedSnapshotterPhasePlanned  UnifiedSnapshotterPhase = "Planned"
	UnifiedSnapshotterPhaseFinished UnifiedSnapshotterPhase = "Finished"
	UnifiedSnapshotterPhaseFailed   UnifiedSnapshotterPhase = "Failed"
)

// UnifiedSnapshotterChildRef mirrors state-snapshotter's storage/v1alpha1.SnapshotChildRef /
// ExcludedObjectRef: a reference within the namespace-local snapshot run tree. Namespace is implicit
// (always the parent snapshot's own namespace) — unlike UnifiedSnapshotterSourceRef, it carries no
// namespace or UID.
type UnifiedSnapshotterChildRef struct {
	// API version of the referenced object.
	APIVersion string `json:"apiVersion"`
	// Kind of the referenced object.
	Kind string `json:"kind"`
	// Name of the referenced object.
	Name string `json:"name"`
}

// UnifiedSnapshotterSourceRef mirrors state-snapshotter's
// storage/v1alpha1.SnapshotSourceObjectRef: the full identity (including UID) of the live source object
// a snapshot captured, published into top-level status.sourceRef. Self-contained so import/restore
// tooling can recreate the source object without joining spec.sourceRef and a separate UID lookup.
type UnifiedSnapshotterSourceRef struct {
	// API version of the referenced object.
	APIVersion string `json:"apiVersion"`
	// Kind of the referenced object.
	Kind string `json:"kind"`
	// Name of the referenced object.
	Name string `json:"name"`
	// Namespace of the referenced object, if namespaced.
	Namespace string `json:"namespace,omitempty"`
	// UID of the referenced object at the time this reference was published.
	UID string `json:"uid,omitempty"`
}

// UnifiedSnapshotterDomainCaptureState mirrors state-snapshotter's
// storage/v1alpha1.DomainSpecificControllerCaptureState (status.captureState.domainSpecificController):
// written by the domain controller via the SDK; the core only reads this structure.
type UnifiedSnapshotterDomainCaptureState struct {
	// ManifestCaptureRequestName is the name of the ManifestCaptureRequest created for this node.
	ManifestCaptureRequestName string `json:"manifestCaptureRequestName,omitempty"`
	// VolumeCaptureRequestName is the name of the VolumeCaptureRequest created for this node's single
	// data leg. Populated only on VirtualDiskSnapshot; always empty on VirtualMachineSnapshot, which
	// carries no data leg of its own.
	VolumeCaptureRequestName string `json:"volumeCaptureRequestName,omitempty"`
	// Phase is the domain capture phase (SDK barrier 1/2 state machine).
	Phase UnifiedSnapshotterPhase `json:"phase,omitempty"`
	// Reason is the machine-readable failure reason, set only when Phase is Failed.
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable progress or failure message.
	Message string `json:"message,omitempty"`
	// ExcludedRefs are this node's direct exclusion vetoes (state-snapshotter.deckhouse.io/exclude label
	// on a child source object). This PoC controller does not implement the veto label, so it always
	// publishes an empty list here (required by the SDK contract to be present, not omitted).
	ExcludedRefs []UnifiedSnapshotterChildRef `json:"excludedRefs"`
}

// UnifiedSnapshotterCommonCaptureState mirrors state-snapshotter's
// storage/v1alpha1.CommonControllerCaptureState (status.captureState.commonController): written by the
// state-snapshotter core; the domain controller only reads it (read-only handoff).
type UnifiedSnapshotterCommonCaptureState struct {
	// ManifestCaptured is the core-owned success latch for the manifest leg. Nil means the leg was not
	// declared yet; false means declared but not yet captured; true means durably captured.
	ManifestCaptured *bool `json:"manifestCaptured,omitempty"`
	// DataCaptured is the core-owned success latch for the data leg, with the same nil/false/true
	// semantics as ManifestCaptured. Always nil on VirtualMachineSnapshot.
	DataCaptured *bool `json:"dataCaptured,omitempty"`
	// ChildSubtreesManifestsPersisted is the core-computed "the subtrees of ALL declared direct children
	// archived their manifests" latch. Not a capture leg: it covers the children only, so "the whole node
	// is persisted" decomposes as ManifestCaptured && ChildSubtreesManifestsPersisted.
	ChildSubtreesManifestsPersisted *bool `json:"childSubtreesManifestsPersisted,omitempty"`
	// SubtreePlanned is a core-computed monotonic recursive latch: true once this node and its whole
	// subtree finished planning (barrier 1). Not a capture leg.
	SubtreePlanned *bool `json:"subtreePlanned,omitempty"`
	// ChildrenSettled is the core-computed "every direct child has gone terminal (captured-OK or
	// failed)" latch. It is NOT a capture leg and does not participate in this node's own
	// AllLegsCaptured/CoreCaptureOutcome — it is the completeness signal a domain controller reads to
	// time a barrier-2 consistency action (e.g. fs unfreeze) that must fire even when a child capture
	// failed. Nil means no direct children (leaf) or not computed yet; true once every direct child is
	// terminal.
	ChildrenSettled *bool `json:"childrenSettled,omitempty"`
}

// UnifiedSnapshotterDataArtifactRef mirrors state-snapshotter's storage/v1alpha1.SnapshotDataArtifactRef:
// the durable data artifact produced by the data path (a VolumeSnapshotContent), never an execution
// request such as a VolumeCaptureRequest.
type UnifiedSnapshotterDataArtifactRef struct {
	// API version of the referenced artifact.
	APIVersion string `json:"apiVersion"`
	// Kind of the referenced artifact.
	Kind string `json:"kind"`
	// Name of the referenced artifact.
	Name string `json:"name"`
	// UID is filled best-effort: the artifact may be referenced before its UID is known.
	UID string `json:"uid,omitempty"`
}

// UnifiedSnapshotterDataBinding mirrors state-snapshotter's storage/v1alpha1.SnapshotDataBinding
// (status.data): the self-contained {source, artifact, volume metadata} block the core mirrors verbatim
// from the bound SnapshotContent onto this snapshot. Written only by the core; the domain controller and
// VirtualDisk provisioning only read it.
//
// It is self-contained on purpose: a snapshot outlives its source PersistentVolumeClaim, so everything
// needed to recreate the volume on restore is recorded here rather than resolved from live objects.
type UnifiedSnapshotterDataBinding struct {
	// SourceRef identifies the captured PersistentVolumeClaim backing this node's data. Distinct from the
	// top-level status.sourceRef, which references the captured live domain object.
	SourceRef UnifiedSnapshotterSourceRef `json:"sourceRef"`
	// ArtifactRef references the cluster-scoped durable data artifact backing the captured data.
	ArtifactRef UnifiedSnapshotterDataArtifactRef `json:"artifactRef"`
	// VolumeMode records the source volume mode (Block or Filesystem). CSI snapshots are mode-agnostic,
	// so it is persisted here to recreate the PersistentVolumeClaim on restore.
	VolumeMode string `json:"volumeMode,omitempty"`
	// FsType records the source filesystem type (Filesystem volumes only).
	FsType string `json:"fsType,omitempty"`
	// AccessModes records the source PersistentVolumeClaim access modes.
	AccessModes []string `json:"accessModes,omitempty"`
	// StorageClassName records the source StorageClass of the captured volume.
	StorageClassName string `json:"storageClassName,omitempty"`
	// Size records the real allocated size of the captured volume, taken from the data artifact's
	// restoreSize. Stored as a resource.Quantity string (e.g. "10Gi").
	Size string `json:"size,omitempty"`
}

// UnifiedSnapshotterCaptureState mirrors state-snapshotter's storage/v1alpha1.CaptureStateStatus
// (status.captureState): an umbrella with exactly one writer per sub-structure. Present only when the
// resource is annotated with AnnUseUnifiedSnapshotter and driven by the unified-snapshotter SDK
// controller; the previous custom Secret-based snapshot controller neither reads nor writes it.
type UnifiedSnapshotterCaptureState struct {
	// CommonController holds the core-written capture-leg success latches. Single writer: core.
	CommonController *UnifiedSnapshotterCommonCaptureState `json:"commonController,omitempty"`
	// DomainSpecificController holds the domain-written planning refs and lifecycle. Single writer: the
	// domain controller, via the SDK.
	DomainSpecificController *UnifiedSnapshotterDomainCaptureState `json:"domainSpecificController,omitempty"`
}
