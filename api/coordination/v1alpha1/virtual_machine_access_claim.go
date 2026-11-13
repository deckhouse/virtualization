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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	VirtualMachineAccessClaimKind     = "VirtualMachineAccessClaim"
	VirtualMachineAccessClaimResource = "virtualmachineaccessclaims"
)

// VirtualMachineAccessClaim requests access to a virtual machine for taking a snapshot of its disks. The
// snapshot that owns the claim creates and releases it; the virtual machine access coordinator grants or
// denies it and is the only component that freezes and unfreezes the guest filesystem.
//
// It is an internal resource: user roles carry no permissions for it.
// +kubebuilder:object:root=true
// +crd-enricher:deckhouse:documentation:examples={apiVersion: coordination.internal.virtualization.deckhouse.io/v1alpha1, kind: VirtualMachineAccessClaim, metadata: {name: vms-7b2c9f4e-1d3a-4c8b-9e2f-5a6d7c8b9e0f}, spec: {virtualMachine: {name: example-vm, uid: 3a9e1b2c-4d5e-4f6a-8b7c-9d0e1f2a3b4c}, access: [Snapshot, Freeze], freezePolicy: Required, timeout: 10m}}
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,singular=virtualmachineaccessclaim
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="VirtualMachineAccessClaim phase."
// +kubebuilder:printcolumn:name="VirtualMachine",type="string",JSONPath=".spec.virtualMachine.name",description="VirtualMachine name."
// +kubebuilder:printcolumn:name="RequesterKind",type="string",JSONPath=".metadata.ownerReferences[?(@.controller==true)].kind",description="Kind of the snapshot that requested access."
// +kubebuilder:printcolumn:name="Requester",type="string",JSONPath=".metadata.ownerReferences[?(@.controller==true)].name",description="Name of the snapshot that requested access."
// +kubebuilder:printcolumn:name="Frozen",type="boolean",JSONPath=".status.frozen",description="Whether the virtual machine filesystem is frozen for this session."
// +kubebuilder:printcolumn:name="Deadline",type="string",JSONPath=".status.deadline",description="Time the session ends unless released earlier."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time of resource creation."
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineAccessClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineAccessClaimSpec   `json:"spec"`
	Status VirtualMachineAccessClaimStatus `json:"status,omitempty"`
}

// VirtualMachineAccessClaimList contains a list of VirtualMachineAccessClaim resources.
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineAccessClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []VirtualMachineAccessClaim `json:"items"`
}

// The spec is fixed at creation except for release: the coordinator decides on the claim once, and a
// different access needs a new claim. The field-level rules cannot see an optional field being added or
// removed, hence the presence rule for freezePolicy here.
// +kubebuilder:validation:XValidation:rule="has(self.freezePolicy) == self.access.exists(a, a == 'Freeze')",message="spec.freezePolicy must be set exactly when spec.access contains Freeze."
// +kubebuilder:validation:XValidation:rule="has(self.freezePolicy) == has(oldSelf.freezePolicy)",message="spec.freezePolicy is immutable."
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.release) || !oldSelf.release || (has(self.release) && self.release)",message="spec.release cannot be unset."

// Requested access to the virtual machine. Cannot be changed after creation, except for `release`.
type VirtualMachineAccessClaimSpec struct {
	// Virtual machine to access. The UID pins the claim to one incarnation of the virtual machine, so a
	// virtual machine recreated under the same name is never accessed through a claim made for its
	// predecessor.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.virtualMachine is immutable."
	VirtualMachine VirtualMachineAccessClaimTarget `json:"virtualMachine"`
	// Access modes requested. Always contains `Snapshot`; `Freeze` is added to freeze the guest filesystem
	// while the snapshot is taken.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=2
	// +listType=set
	// +kubebuilder:validation:XValidation:rule="self.exists(a, a == 'Snapshot')",message="spec.access must contain Snapshot."
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.access is immutable."
	Access []AccessMode `json:"access"`
	// What happens when the filesystem cannot be frozen. Set only when access contains `Freeze`:
	// * `Required`: the claim is denied.
	// * `BestEffort`: the access is granted without the freeze, and the snapshot is not consistent.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.freezePolicy is immutable."
	FreezePolicy *FreezePolicy `json:"freezePolicy,omitempty"`
	// Session deadline, counted from the moment access is granted. Past it the coordinator unfreezes the
	// virtual machine and aborts the session. At most 30m, as it also bounds how long the guest may stay
	// frozen without the coordinator. A session on a stopped virtual machine has no deadline.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`
	// +kubebuilder:validation:XValidation:rule="duration(self) > duration('0s') && duration(self) <= duration('30m')",message="spec.timeout must be greater than zero and not exceed 30m."
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.timeout is immutable."
	Timeout metav1.Duration `json:"timeout"`
	// Set by the snapshot once the access is no longer needed. Cannot be unset.
	// +optional
	Release bool `json:"release,omitempty"`
}

// Virtual machine in the namespace of the claim.
type VirtualMachineAccessClaimTarget struct {
	// Name of the virtual machine.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// UID of the virtual machine.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	UID types.UID `json:"uid"`
}

// Access to the virtual machine:
// * `Snapshot`: taking a snapshot of the virtual machine disks.
// * `Freeze`: freezing the guest filesystem.
// +kubebuilder:validation:Enum={Snapshot,Freeze}
type AccessMode string

const (
	AccessModeSnapshot AccessMode = "Snapshot"
	AccessModeFreeze   AccessMode = "Freeze"
)

// Policy applied when the filesystem cannot be frozen.
// +kubebuilder:validation:Enum={Required,BestEffort}
type FreezePolicy string

const (
	FreezePolicyRequired   FreezePolicy = "Required"
	FreezePolicyBestEffort FreezePolicy = "BestEffort"
)

type VirtualMachineAccessClaimStatus struct {
	Phase AccessClaimPhase `json:"phase,omitempty"`
	// UID of the virtual machine instance the session is bound to. Empty for a stopped virtual machine.
	VirtualMachineInstanceUID types.UID `json:"virtualMachineInstanceUID,omitempty"`
	// Whether the virtual machine filesystem was frozen for this session.
	Frozen bool `json:"frozen,omitempty"`
	// Time the access was granted.
	GrantedAt *metav1.Time `json:"grantedAt,omitempty"`
	// Time the session ends unless released earlier. Not set for a session on a stopped virtual machine,
	// which ends when the machine starts.
	Deadline *metav1.Time `json:"deadline,omitempty"`
	// The latest available observations of the claim's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Resource generation last processed by the coordinator.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Current phase of the claim:
// * `Pending`: the coordinator is granting the access, for example waiting for the filesystem to freeze.
// * `Granted`: the access is granted.
// * `Denied`: the access cannot be granted.
// * `Aborted`: the session was interrupted before release.
// * `Completed`: the access was released and the session went through without violations.
// +kubebuilder:validation:Enum={Pending,Granted,Denied,Aborted,Completed}
type AccessClaimPhase string

const (
	AccessClaimPhasePending   AccessClaimPhase = "Pending"
	AccessClaimPhaseGranted   AccessClaimPhase = "Granted"
	AccessClaimPhaseDenied    AccessClaimPhase = "Denied"
	AccessClaimPhaseAborted   AccessClaimPhase = "Aborted"
	AccessClaimPhaseCompleted AccessClaimPhase = "Completed"
)
