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
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk/transform"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Compiler recursively compiles restore-ready manifests for a VirtualMachineSnapshot subtree (itself
// plus its VirtualDiskSnapshot children), without a dedicated aggregated apiserver: each node's own raw
// manifest comes from the state-snapshotter core's existing manifests-download subresource
// (ManifestClient); recursion, the readiness gate, sanitization and the domain restore transform all run
// in-process, called directly by the VMSOP reconciler.
type Compiler struct {
	// Reader reads VirtualMachineSnapshot/VirtualDiskSnapshot objects: the fail-closed readiness gate and
	// the VirtualMachineSnapshot's declared children (status.childrenSnapshotRefs).
	Reader client.Reader
	// Manifests fetches each node's own base manifest from the state-snapshotter core.
	Manifests *ManifestClient
}

func NewCompiler(reader client.Reader, manifests *ManifestClient) *Compiler {
	return &Compiler{Reader: reader, Manifests: manifests}
}

// CompileVirtualMachineSnapshot compiles restore-ready manifests for the VirtualMachineSnapshot subtree
// rooted at name, in post-order (children before parent) so a straight sequential apply creates disks
// before the VirtualMachine that references them via spec.dataSource.
func (c *Compiler) CompileVirtualMachineSnapshot(ctx context.Context, namespace, name, targetNamespace string) ([]unstructured.Unstructured, error) {
	return c.compileNode(ctx, v1alpha2.VirtualMachineSnapshotResource, namespace, name, targetNamespace, map[string]struct{}{})
}

// compileNode compiles one snapshot node: it enforces the readiness gate, recurses into declared
// children first (post-order), then fetches and transforms this node's own base manifest. visited
// guards against a run-tree cycle.
func (c *Compiler) compileNode(ctx context.Context, resource, namespace, name, targetNamespace string, visited map[string]struct{}) ([]unstructured.Unstructured, error) {
	key := resource + "/" + name
	if _, ok := visited[key]; ok {
		return nil, fmt.Errorf("snapshot run-tree cycle at %s/%s", resource, name)
	}
	visited[key] = struct{}{}

	kind, childRefs, boundSnapshotContentName, err := c.loadNode(ctx, resource, namespace, name)
	if err != nil {
		return nil, err
	}

	out := make([]unstructured.Unstructured, 0)
	for _, ref := range childRefs {
		childResource, ok := resourceForKind(ref.Kind)
		if !ok {
			return nil, fmt.Errorf("snapshot %s %s/%s has unsupported child kind %q", resource, namespace, name, ref.Kind)
		}
		childObjs, err := c.compileNode(ctx, childResource, namespace, ref.Name, targetNamespace, visited)
		if err != nil {
			return nil, err
		}
		out = append(out, childObjs...)
	}

	base, err := c.Manifests.NodeBaseManifests(ctx, boundSnapshotContentName)
	if err != nil {
		return nil, err
	}
	node := &transform.RestoreNode{
		SnapshotRef: storagev1alpha1.ObjectRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
		},
	}
	tr := Transformer{}
	for _, obj := range base {
		sanitized := SanitizeForRestore(obj, targetNamespace)
		if _, err := tr.TransformObject(node, &sanitized, nil); err != nil {
			return nil, fmt.Errorf("transform %s %s/%s manifest: %w", resource, namespace, name, err)
		}
		out = append(out, sanitized)
	}
	return out, nil
}

// loadNode reads the addressed snapshot object, enforces the fail-closed readiness gate (a snapshot's
// Ready mirrors its bound SnapshotContent.Ready, so restoring from a not-Ready node would compile stale
// or mid-recapture data), and returns its kind, direct children (VirtualDiskSnapshot is a leaf: no
// children).
func (c *Compiler) loadNode(ctx context.Context, resource, namespace, name string) (kind string, children []v1alpha2.UnifiedSnapshotterChildRef, boundSnapshotContentName string, err error) {
	switch resource {
	case v1alpha2.VirtualMachineSnapshotResource:
		obj := &v1alpha2.VirtualMachineSnapshot{}
		if err := c.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
			return "", nil, "", fmt.Errorf("get VirtualMachineSnapshot %s/%s: %w", namespace, name, err)
		}
		if !meta.IsStatusConditionTrue(obj.Status.Conditions, v1alpha2.UnifiedSnapshotterConditionReady) {
			return "", nil, "", fmt.Errorf("VirtualMachineSnapshot %s/%s is not Ready", namespace, name)
		}
		if obj.Status.BoundSnapshotContentName == "" {
			return "", nil, "", fmt.Errorf("VirtualMachineSnapshot %s/%s has no bound SnapshotContent yet", namespace, name)
		}
		return v1alpha2.VirtualMachineSnapshotKind, obj.Status.ChildrenSnapshotRefs, obj.Status.BoundSnapshotContentName, nil
	case v1alpha2.VirtualDiskSnapshotResource:
		obj := &v1alpha2.VirtualDiskSnapshot{}
		if err := c.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, obj); err != nil {
			return "", nil, "", fmt.Errorf("get VirtualDiskSnapshot %s/%s: %w", namespace, name, err)
		}
		if !meta.IsStatusConditionTrue(obj.Status.Conditions, v1alpha2.UnifiedSnapshotterConditionReady) {
			return "", nil, "", fmt.Errorf("VirtualDiskSnapshot %s/%s is not Ready", namespace, name)
		}
		if obj.Status.BoundSnapshotContentName == "" {
			return "", nil, "", fmt.Errorf("VirtualDiskSnapshot %s/%s has no bound SnapshotContent yet", namespace, name)
		}
		return v1alpha2.VirtualDiskSnapshotKind, nil, obj.Status.BoundSnapshotContentName, nil
	default:
		return "", nil, "", fmt.Errorf("unsupported snapshot resource %q", resource)
	}
}

// resourceForKind maps a snapshot Kind (as carried in status.childrenSnapshotRefs) to its lowercase
// plural resource. ok is false for any kind this compiler does not know how to recurse into.
func resourceForKind(kind string) (resource string, ok bool) {
	switch kind {
	case v1alpha2.VirtualMachineSnapshotKind:
		return v1alpha2.VirtualMachineSnapshotResource, true
	case v1alpha2.VirtualDiskSnapshotKind:
		return v1alpha2.VirtualDiskSnapshotResource, true
	default:
		return "", false
	}
}
