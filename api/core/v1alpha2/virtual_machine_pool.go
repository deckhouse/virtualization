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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VirtualMachinePoolKind     = "VirtualMachinePool"
	VirtualMachinePoolResource = "virtualmachinepools"
)

// VirtualMachinePool declaratively manages a group of identical virtual machines:
// it keeps the requested number of replicas, scales via the standard `scale`
// subresource, and reuses "heavy" disks across replica generations.
//
// The resource is available only in paid editions (EE/SE+) and is gated behind
// the `VirtualMachinePool` module feature gate.
//
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:resource:categories={virtualization},scope=Namespaced,shortName={vmpool,vmpools},singular=virtualmachinepool
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas",description="Current number of pool members (including Terminating)."
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="Number of members ready to serve."
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time of resource creation."
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachinePoolSpec   `json:"spec"`
	Status VirtualMachinePoolStatus `json:"status,omitempty"`
}

// VirtualMachinePoolList contains a list of VirtualMachinePool resources.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VirtualMachinePool `json:"items"`
}

// VirtualMachinePoolSpec is the desired state of a VirtualMachinePool.
//
// The disks a replica gets are declared in two places that must stay in sync:
// virtualDiskTemplates describes each per-replica disk, and the template's
// blockDeviceRefs references those disks (by name, kind VirtualDisk) to set the
// boot order and interleave shared images. The rule below enforces a bijection —
// every template is referenced exactly once, and every VirtualDisk reference
// resolves to a template — so neither list can carry a dangling entry.
//
// +kubebuilder:validation:XValidation:rule="has(self.virtualMachineTemplate.spec) && has(self.virtualMachineTemplate.spec.blockDeviceRefs) && self.virtualDiskTemplates.all(t, self.virtualMachineTemplate.spec.blockDeviceRefs.exists(r, r.kind == 'VirtualDisk' && r.name == t.name)) && self.virtualMachineTemplate.spec.blockDeviceRefs.filter(r, r.kind == 'VirtualDisk').all(r, self.virtualDiskTemplates.exists(t, t.name == r.name)) && self.virtualMachineTemplate.spec.blockDeviceRefs.filter(r, r.kind == 'VirtualDisk').size() == self.virtualDiskTemplates.size()",message="each virtualDiskTemplates entry must be referenced exactly once by a VirtualDisk entry in virtualMachineTemplate.spec.blockDeviceRefs, and every VirtualDisk reference must name a virtualDiskTemplates entry"
type VirtualMachinePoolSpec struct {
	// Replicas is the desired number of virtual machines in the pool.
	//
	// The field is written only by its owner — an autoscaler or a human via the
	// `scale` subresource, or by the addressed scale-down handler. The controller
	// never writes it. Bounds are held by the autoscaler; the hard ceiling is the
	// namespace ResourceQuota.
	//
	// +kubebuilder:validation:Minimum=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ScaleDownPolicy chooses how a replica is picked when the pool is scaled down
	// anonymously through the `scale` subresource. It is required and has no
	// default, forcing a conscious choice between "any replica may be killed" and
	// "only addressed removal is allowed".
	//
	//   - `NewestFirst` — anonymous scale-down is allowed; the youngest replicas
	//     (least accumulated state) are removed first.
	//   - `OldestFirst` — anonymous scale-down is allowed; the oldest replicas are
	//     removed first (faster rotation).
	//   - `Explicit` — anonymous scale-down through `scale` is rejected by a
	//     webhook; replicas can be removed only by address. For "busy" workloads
	//     such as CI runners and VDI.
	//
	// +kubebuilder:validation:Enum=NewestFirst;OldestFirst;Explicit
	ScaleDownPolicy ScaleDownPolicy `json:"scaleDownPolicy"`

	// VirtualMachineTemplate is the template every replica is stamped from. Its
	// `spec` is an ordinary VirtualMachineSpec, so a replica is no different from a
	// manually created virtual machine.
	VirtualMachineTemplate VirtualMachineTemplateSpec `json:"virtualMachineTemplate"`

	// VirtualDiskTemplates describes each per-replica disk (reclaim policy, size,
	// data source). Names are unique within the pool (list-map key) and every
	// template must be referenced by a VirtualDisk entry in
	// virtualMachineTemplate.spec.blockDeviceRefs, which sets the boot order (see
	// the bijection rule on the spec). A disk with reclaim Delete belongs to its
	// VirtualMachine and is removed with it; a disk with reclaim Retain belongs to
	// the pool, outlives the replica and is reused on a later scale-up.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +listType=map
	// +listMapKey=name
	VirtualDiskTemplates []VirtualDiskTemplateSpec `json:"virtualDiskTemplates"`
}

// VirtualDiskTemplateSpec describes a per-replica disk.
type VirtualDiskTemplateSpec struct {
	// Name identifies the disk template within the pool. It is a DNS-1123 label
	// (no dots), because it is embedded into VirtualDisk names.
	//
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// Reclaim controls what happens to the disk when its replica is removed.
	//
	// +optional
	Reclaim VirtualDiskReclaim `json:"reclaim,omitempty"`

	// Spec is the desired state of the disk (an ordinary VirtualDiskSpec).
	Spec VirtualDiskSpec `json:"spec"`
}

// VirtualDiskReclaimPolicy selects the fate of a per-replica disk on scale-down.
type VirtualDiskReclaimPolicy string

const (
	// VirtualDiskReclaimDelete removes the disk together with its replica (owner
	// is the VirtualMachine). This is the default.
	VirtualDiskReclaimDelete VirtualDiskReclaimPolicy = "Delete"
	// VirtualDiskReclaimRetain keeps the disk (owner is the pool); it is reused on
	// the next scale-up.
	VirtualDiskReclaimRetain VirtualDiskReclaimPolicy = "Retain"
)

// VirtualDiskReclaim is the reclaim policy and warm-buffer settings of a disk
// template.
//
// +kubebuilder:validation:XValidation:rule="self.onScaleDown == 'Retain' || (self.keep == 0 && !has(self.ttl))",message="keep and ttl are only valid with onScaleDown: Retain"
// +kubebuilder:validation:XValidation:rule="self.keep == 0 || has(self.ttl)",message="keep requires ttl; without ttl free disks are never garbage-collected, so keep would have no effect"
type VirtualDiskReclaim struct {
	// OnScaleDown is Delete (default) or Retain.
	//
	// +kubebuilder:validation:Enum=Delete;Retain
	// +kubebuilder:default=Delete
	// +optional
	OnScaleDown VirtualDiskReclaimPolicy `json:"onScaleDown,omitempty"`

	// Keep is the number of free (Retain) disks always kept warm for fast
	// scale-up; these are immune to the ttl. Only meaningful with Retain.
	//
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	// +optional
	Keep int32 `json:"keep,omitempty"`

	// TTL is how long a free disk lives beyond the warm buffer before it is
	// garbage-collected. Only meaningful with Retain.
	//
	// +optional
	TTL *metav1.Duration `json:"ttl,omitempty"`
}

// ScaleDownPolicy selects which replica is removed on anonymous scale-down.
type ScaleDownPolicy string

const (
	ScaleDownPolicyNewestFirst ScaleDownPolicy = "NewestFirst"
	ScaleDownPolicyOldestFirst ScaleDownPolicy = "OldestFirst"
	ScaleDownPolicyExplicit    ScaleDownPolicy = "Explicit"
)

// VirtualMachineTemplateSpec describes the metadata and spec a pool replica is
// created with.
type VirtualMachineTemplateSpec struct {
	// Metadata applied to every replica. Arbitrary user labels and annotations are
	// allowed; the controller adds its managed pool labels on top. A curated
	// struct (not the full ObjectMeta) so the CRD schema exposes labels and
	// annotations instead of an opaque object.
	//
	// +optional
	Metadata VirtualMachineTemplateMetadata `json:"metadata,omitempty"`

	// Spec of the virtual machine that backs each replica.
	//
	// +optional
	Spec VirtualMachineSpec `json:"spec,omitempty"`
}

// VirtualMachineTemplateMetadata is the subset of object metadata a pool
// stamps onto each replica.
type VirtualMachineTemplateMetadata struct {
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// VirtualMachinePoolStatus is the observed state of a VirtualMachinePool.
type VirtualMachinePoolStatus struct {
	// ObservedGeneration is the generation of the spec the controller has processed.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Replicas is the number of existing members, including those in Terminating:
	// such a machine still occupies resources, so it is real capacity, not a phantom.
	//
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of members ready to serve (Terminating excluded).
	//
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// DesiredTemplateHash is the hash of the current virtualMachineTemplate — the
	// revision the controller is converging replicas to (cf. updateRevision on a
	// StatefulSet).
	//
	// +optional
	DesiredTemplateHash string `json:"desiredTemplateHash,omitempty"`

	// UpdatedReplicas is the number of replicas effectively on DesiredTemplateHash
	// (fully synced).
	//
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// RestartPendingReplicas is the number of replicas patched to the new template
	// whose disruptive part still awaits a restart.
	//
	// +optional
	RestartPendingReplicas int32 `json:"restartPendingReplicas,omitempty"`

	// Selector is the label selector the controller publishes for the `scale`
	// subresource; HPA/KEDA read it themselves.
	//
	// +optional
	Selector string `json:"selector,omitempty"`

	// Conditions describe the current state of the pool.
	//
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}
