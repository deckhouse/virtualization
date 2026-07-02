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
	// Standard object metadata applied to every replica. Arbitrary user labels and
	// annotations are allowed; the controller adds its managed pool labels on top.
	//
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec of the virtual machine that backs each replica.
	//
	// +optional
	Spec VirtualMachineSpec `json:"spec,omitempty"`
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
