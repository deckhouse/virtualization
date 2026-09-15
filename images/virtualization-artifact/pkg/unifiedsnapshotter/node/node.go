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

// Package node reads a VirtualMachineSnapshot or VirtualDiskSnapshot as a node of a state-snapshotter
// run tree, and resolves the cluster-scoped SnapshotContent that backs it.
//
// All three per-CR aggregated subresources need the same two things and must agree on them exactly: what
// the addressed snapshot is (kind, identity, mode, bound content, declared children) and whether the
// content it names really belongs to it.
//
// # Why the second question has to be asked
//
// A request names a snapshot in a namespace, and the caller's RBAC is checked against that — their own
// object, in their own namespace. But the manifests do not live there. They live in a SnapshotContent,
// which is CLUSTER-SCOPED and holds the captured state of whichever namespace it was captured from, and
// the only thing tying the two together is status.boundSnapshotContentName on the snapshot: a plain
// string field, on a namespaced object, that points across the namespace boundary.
//
// The aggregated apiserver reads and writes that content with its OWN service account, which can reach
// every content in the cluster — it has to, since it serves every namespace. So whoever can set that
// string picks which content the apiserver fetches or writes into on their behalf, while the
// authorization that was actually performed only ever covered the namespaced snapshot they addressed.
// Point it at a content captured from another namespace and manifests-download hands back that
// namespace's objects (the captured provisioner Secret among them), manifests-with-data-restoration
// compiles them into something applyable, and manifests-and-children-refs-upload writes into them. The
// apiserver is the confused deputy; the string is the caller's half of the confusion.
//
// # What closes it
//
// The binding is only trusted when it is mutual. ResolveBoundContent reads the content and requires its
// spec.snapshotRef to name the addressed snapshot back — apiVersion, kind, namespace, name, and the uid
// when the ref carries one. That ref is written by the core's binder when it binds the content and is
// immutable while the owning snapshot is alive, so it is not the caller's to set; the forward pointer is
// theirs, the back-pointer is not, and a content is reachable only where both agree. The uid is what
// keeps the agreement from surviving a delete: a snapshot recreated under the same name is a different
// object, and the content that belonged to the old one stops matching.
//
// The check runs before the content is read and before anything is written into it, on every one of the
// three subresources and on the VirtualDisk and VirtualMachineOperation restore paths — which is why the
// rule lives here, in one function, rather than being restated at each of them. A second copy that
// compared one field fewer would reopen exactly the hole above, and it would do so silently.
//
// Under the roles this module ships, status is a real subresource on both kinds and none of the rbacv2
// capabilities grant virtualmachinesnapshots/status or virtualdisksnapshots/status, so an ordinary
// namespace manager cannot set the pointer in the first place. That is worth knowing and worth not
// relying on: it makes the hole a property of how the roles happen to be drawn today, and this check is
// what makes it a property of the API instead.
package node

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Node is everything the aggregated subresources need to know about one snapshot node. It is read in a
// single Get so that mode, bound content and children cannot come from three different revisions of the
// object.
type Node struct {
	// APIVersion, Kind, Namespace, Name and UID are the node's full identity, as the bound
	// SnapshotContent's spec.snapshotRef must reproduce it.
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
	UID        types.UID

	// Mode is spec.mode. The upload facade accepts only Import; capture-mode manifests are written by
	// the core from a real capture and must not be overwritten from a request body.
	Mode v1alpha2.UnifiedSnapshotterMode
	// ContentName is status.boundSnapshotContentName, empty until the core's binder binds this node.
	ContentName string
	// Ready mirrors the bound SnapshotContent's readiness. Written by the core; only restore gates on it.
	Ready bool
	// Children are the node's declared direct children (status.childrenSnapshotRefs). Always empty on a
	// VirtualDiskSnapshot, which is a data-bearing leaf.
	Children []v1alpha2.UnifiedSnapshotterChildRef
	// KeepIPAddress is spec.keepIPAddress of a VirtualMachineSnapshot, and the zero value elsewhere.
	KeepIPAddress v1alpha2.KeepIPAddress
}

// IsImport reports whether this node is an import target rather than a capture.
func (n Node) IsImport() bool {
	return n.Mode == v1alpha2.UnifiedSnapshotterModeImport
}

// ResourceForKind maps a snapshot Kind, as carried in status.childrenSnapshotRefs, to its lowercase
// plural resource. ok is false for any kind outside this domain: a node of ours may only reference our
// own snapshot kinds, and both the restore recursion and the upload facade fail closed on anything else
// rather than silently dropping a subtree.
func ResourceForKind(kind string) (resource string, ok bool) {
	switch kind {
	case v1alpha2.VirtualMachineSnapshotKind:
		return v1alpha2.VirtualMachineSnapshotResource, true
	case v1alpha2.VirtualDiskSnapshotKind:
		return v1alpha2.VirtualDiskSnapshotResource, true
	default:
		return "", false
	}
}

// Load reads the addressed snapshot node. reader must be uncached: mode and
// status.boundSnapshotContentName gate a write and a read of captured manifests respectively, and a
// stale view of either would let a request through against the wrong revision of the object.
func Load(ctx context.Context, reader client.Reader, resource, namespace, name string) (Node, error) {
	key := types.NamespacedName{Namespace: namespace, Name: name}

	switch resource {
	case v1alpha2.VirtualMachineSnapshotResource:
		obj := &v1alpha2.VirtualMachineSnapshot{}
		if err := reader.Get(ctx, key, obj); err != nil {
			return Node{}, fmt.Errorf("get VirtualMachineSnapshot %s/%s: %w", namespace, name, err)
		}
		return Node{
			APIVersion:    v1alpha2.SchemeGroupVersion.String(),
			Kind:          v1alpha2.VirtualMachineSnapshotKind,
			Namespace:     namespace,
			Name:          name,
			UID:           obj.UID,
			Mode:          obj.Spec.Mode,
			ContentName:   obj.Status.BoundSnapshotContentName,
			Ready:         meta.IsStatusConditionTrue(obj.Status.Conditions, v1alpha2.UnifiedSnapshotterConditionReady),
			Children:      obj.Status.ChildrenSnapshotRefs,
			KeepIPAddress: obj.Spec.KeepIPAddress,
		}, nil

	case v1alpha2.VirtualDiskSnapshotResource:
		obj := &v1alpha2.VirtualDiskSnapshot{}
		if err := reader.Get(ctx, key, obj); err != nil {
			return Node{}, fmt.Errorf("get VirtualDiskSnapshot %s/%s: %w", namespace, name, err)
		}
		return Node{
			APIVersion:  v1alpha2.SchemeGroupVersion.String(),
			Kind:        v1alpha2.VirtualDiskSnapshotKind,
			Namespace:   namespace,
			Name:        name,
			UID:         obj.UID,
			Mode:        obj.Spec.Mode,
			ContentName: obj.Status.BoundSnapshotContentName,
			Ready:       meta.IsStatusConditionTrue(obj.Status.Conditions, v1alpha2.UnifiedSnapshotterConditionReady),
		}, nil

	default:
		return Node{}, fmt.Errorf("unsupported snapshot resource %q", resource)
	}
}

// ResolveBoundContent returns the name of the SnapshotContent backing n, once that content has proved it
// belongs to n. An unbound node answers NotFound and a content that does not point back answers
// Forbidden — permanently, because no retry can turn a foreign content into this node's own.
func ResolveBoundContent(ctx context.Context, reader client.Reader, n Node) (string, error) {
	if n.ContentName == "" {
		return "", apierrors.NewNotFound(
			storagev1alpha1.SchemeGroupVersion.WithResource("snapshotcontents").GroupResource(),
			fmt.Sprintf("%s %s/%s has no status.boundSnapshotContentName yet", n.Kind, n.Namespace, n.Name),
		)
	}

	content := &storagev1alpha1.SnapshotContent{}
	if err := reader.Get(ctx, types.NamespacedName{Name: n.ContentName}, content); err != nil {
		return "", fmt.Errorf("get SnapshotContent %q bound by %s %s/%s: %w", n.ContentName, n.Kind, n.Namespace, n.Name, err)
	}

	if msg := backRefMismatch(content.Spec.SnapshotRef, n); msg != "" {
		return "", apierrors.NewForbidden(
			storagev1alpha1.SchemeGroupVersion.WithResource("snapshotcontents").GroupResource(),
			n.ContentName,
			fmt.Errorf("SnapshotContent %q does not belong to %s %s/%s: %s", n.ContentName, n.Kind, n.Namespace, n.Name, msg),
		)
	}

	return n.ContentName, nil
}

// backRefMismatch returns why ref fails to identify n, or "" when it identifies it exactly. The UID is
// compared only when the ref carries one: the core fills it for a node it bound itself, and leaves it
// empty on paths that have no UID to record, where the remaining four fields are the whole answer.
func backRefMismatch(ref *storagev1alpha1.SnapshotSubjectRef, n Node) string {
	switch {
	case ref == nil:
		return "its spec.snapshotRef is absent"
	case ref.APIVersion != n.APIVersion:
		return fmt.Sprintf("its spec.snapshotRef names apiVersion %q, not %q", ref.APIVersion, n.APIVersion)
	case ref.Kind != n.Kind:
		return fmt.Sprintf("its spec.snapshotRef names kind %q, not %q", ref.Kind, n.Kind)
	case ref.Namespace != n.Namespace:
		return fmt.Sprintf("its spec.snapshotRef names namespace %q, not %q", ref.Namespace, n.Namespace)
	case ref.Name != n.Name:
		return fmt.Sprintf("its spec.snapshotRef names %q, not %q", ref.Name, n.Name)
	case ref.UID != "" && ref.UID != n.UID:
		return fmt.Sprintf("its spec.snapshotRef names uid %q, not %q", ref.UID, n.UID)
	default:
		return ""
	}
}

// EmptyObject returns an empty typed object for resource, named by the caller. Status patches address an
// object rather than send one, so this carries no content of its own.
func EmptyObject(resource string) (client.Object, error) {
	switch resource {
	case v1alpha2.VirtualMachineSnapshotResource:
		return &v1alpha2.VirtualMachineSnapshot{}, nil
	case v1alpha2.VirtualDiskSnapshotResource:
		return &v1alpha2.VirtualDiskSnapshot{}, nil
	default:
		return nil, fmt.Errorf("unsupported snapshot resource %q", resource)
	}
}

// KindForResource maps a lowercase plural resource to its Kind. ok is false for anything outside this
// domain's snapshot kinds.
func KindForResource(resource string) (kind string, ok bool) {
	switch resource {
	case v1alpha2.VirtualMachineSnapshotResource:
		return v1alpha2.VirtualMachineSnapshotKind, true
	case v1alpha2.VirtualDiskSnapshotResource:
		return v1alpha2.VirtualDiskSnapshotKind, true
	default:
		return "", false
	}
}
