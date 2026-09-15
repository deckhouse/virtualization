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

package restore

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk/transform"
	snapshotnode "github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/node"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var ErrSnapshotNotReady = errors.New("snapshot is not ready")

// Compiler recursively compiles restore-ready manifests for a VirtualMachineSnapshot subtree (itself
// plus its VirtualDiskSnapshot children), without a dedicated aggregated apiserver: each node's own raw
// manifest comes from the state-snapshotter core's existing manifests-download subresource
// (ManifestClient); recursion, the readiness gate, sanitization and the domain restore transform all run
// in-process, called directly by the VMSOP reconciler.
type Compiler struct {
	// Reader reads VirtualMachineSnapshot/VirtualDiskSnapshot objects — the fail-closed readiness gate
	// and the VirtualMachineSnapshot's declared children (status.childrenSnapshotRefs) — and the
	// cluster-scoped SnapshotContent each node names, to check that it names the node back. It must be
	// uncached: all three decide what a caller is allowed to read.
	Reader client.Reader
	// Manifests fetches each node's own base manifest from the state-snapshotter core.
	Manifests NodeManifestFetcher
}

// NodeManifestFetcher is the one call the compiler makes into the state-snapshotter core: the own-node
// (non-recursive) captured manifests of the snapshot bound to contentName.
type NodeManifestFetcher interface {
	NodeBaseManifests(ctx context.Context, contentName string) ([]unstructured.Unstructured, error)
}

func NewCompiler(reader client.Reader, manifests NodeManifestFetcher) *Compiler {
	return &Compiler{Reader: reader, Manifests: manifests}
}

// CompileVirtualMachineSnapshot compiles restore-ready manifests for the VirtualMachineSnapshot subtree
// rooted at name, in post-order (children before parent) so a straight sequential apply creates disks
// before the VirtualMachine that references them via spec.dataSource.
func (c *Compiler) CompileVirtualMachineSnapshot(ctx context.Context, namespace, name string) ([]unstructured.Unstructured, error) {
	return c.compileNode(ctx, v1alpha2.VirtualMachineSnapshotResource, namespace, name, map[string]struct{}{})
}

func (c *Compiler) CompileVirtualDiskSnapshot(ctx context.Context, namespace, name string) ([]unstructured.Unstructured, error) {
	return c.compileNode(ctx, v1alpha2.VirtualDiskSnapshotResource, namespace, name, map[string]struct{}{})
}

func (c *Compiler) CompileSubtree(ctx context.Context, resource, namespace, name string) ([]unstructured.Unstructured, error) {
	switch resource {
	case v1alpha2.VirtualMachineSnapshotResource:
		return c.CompileVirtualMachineSnapshot(ctx, namespace, name)
	case v1alpha2.VirtualDiskSnapshotResource:
		return c.CompileVirtualDiskSnapshot(ctx, namespace, name)
	default:
		return nil, fmt.Errorf("unsupported snapshot resource %q", resource)
	}
}

// compileNode compiles one snapshot node: it enforces the readiness gate, recurses into declared
// children first (post-order), then fetches and transforms this node's own base manifest. visited
// guards against a run-tree cycle.
func (c *Compiler) compileNode(ctx context.Context, resource, namespace, name string, visited map[string]struct{}) ([]unstructured.Unstructured, error) {
	key := resource + "/" + name
	if _, ok := visited[key]; ok {
		return nil, fmt.Errorf("snapshot run-tree cycle at %s/%s", resource, name)
	}
	visited[key] = struct{}{}

	n, err := snapshotnode.Load(ctx, c.Reader, resource, namespace, name)
	if err != nil {
		return nil, err
	}
	// Fail-closed readiness gate: a snapshot's Ready mirrors its bound SnapshotContent's, so compiling
	// from a node that is not Ready would emit stale or mid-recapture data as if it were the capture. A
	// Ready node that is not bound yet is the same answer for a different reason — binding is a core-side
	// step, so the gap is a race and not a corruption.
	if !n.Ready || n.ContentName == "" {
		return nil, fmt.Errorf("%s %s/%s: %w", n.Kind, namespace, name, ErrSnapshotNotReady)
	}

	out := make([]unstructured.Unstructured, 0)
	for _, ref := range n.Children {
		childResource, ok := snapshotnode.ResourceForKind(ref.Kind)
		if !ok {
			return nil, fmt.Errorf("snapshot %s %s/%s has unsupported child kind %q", resource, namespace, name, ref.Kind)
		}
		childObjs, err := c.compileNode(ctx, childResource, namespace, ref.Name, visited)
		if err != nil {
			return nil, err
		}
		out = append(out, childObjs...)
	}

	// The content is resolved through the same handshake the download and upload subresources use: the
	// content this node names must name it back. status.boundSnapshotContentName is a status field on a
	// namespaced object, so without that check a tenant able to write it could have restore compile from
	// another namespace's captured manifests.
	contentName, err := snapshotnode.ResolveBoundContent(ctx, c.Reader, n)
	if err != nil {
		return nil, err
	}
	base, err := c.Manifests.NodeBaseManifests(ctx, contentName)
	if err != nil {
		return nil, err
	}
	restoreNode := &transform.RestoreNode{
		SnapshotRef: storagev1alpha1.ObjectRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       n.Kind,
			Name:       name,
			Namespace:  namespace,
		},
	}
	if err := applyCapturedMACAddresses(base); err != nil {
		return nil, fmt.Errorf("restore the MAC addresses of %s %s/%s: %w", resource, namespace, name, err)
	}

	tr := Transformer{}
	nodeObjs := make([]unstructured.Unstructured, 0, len(base))
	for _, obj := range base {
		// Pin the object to the namespace of the node addressed, overwriting whatever the captured
		// manifest carries. It does carry one: a capture records metadata.namespace as it stood, and an
		// import stores the manifests the client uploaded verbatim, so for an imported node the field
		// names the namespace the original was captured from — a foreign one. This line is what keeps a
		// compiled restore inside the namespace it was asked for, so it is not the formality it looks
		// like: dropping it would emit objects addressed at somebody else's namespace.
		obj.SetNamespace(namespace)
		if err := applyKeepIPAddress(&obj, n.KeepIPAddress); err != nil {
			return nil, err
		}
		sanitized := SanitizeForRestore(obj, namespace)
		if _, err := tr.TransformObject(restoreNode, &sanitized, nil); err != nil {
			return nil, fmt.Errorf("transform %s %s/%s manifest: %w", resource, namespace, name, err)
		}
		nodeObjs = append(nodeObjs, sanitized)
	}
	if err := linkCapturedIPAddress(nodeObjs); err != nil {
		return nil, err
	}
	return append(out, virtualMachineLast(nodeObjs)...), nil
}

// virtualMachineLast moves the VirtualMachine to the end of one node's manifests, keeping everything else in capture order.
func virtualMachineLast(objs []unstructured.Unstructured) []unstructured.Unstructured {
	ordered := make([]unstructured.Unstructured, 0, len(objs))
	var machines []unstructured.Unstructured
	for _, obj := range objs {
		if isVirtualMachine(obj) {
			machines = append(machines, obj)
			continue
		}
		ordered = append(ordered, obj)
	}
	return append(ordered, machines...)
}
