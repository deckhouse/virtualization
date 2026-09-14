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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	testNamespace = "ns"
	vmsContent    = "content-vms"
	vdsContent    = "content-vds"
)

// fakeManifests serves per-content manifests and records the order nodes were fetched in, so a test can
// assert the post-order walk without reaching into the compiler.
type fakeManifests struct {
	byContent map[string][]unstructured.Unstructured
	fetched   []string
	err       error
}

func (f *fakeManifests) NodeBaseManifests(_ context.Context, contentName string) ([]unstructured.Unstructured, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.fetched = append(f.fetched, contentName)
	return f.byContent[contentName], nil
}

func readyConditions() []metav1.Condition {
	return []metav1.Condition{{
		Type:   v1alpha2.UnifiedSnapshotterConditionReady,
		Status: metav1.ConditionTrue,
		Reason: "Captured",
	}}
}

func obj(kind, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": v1alpha2.SchemeGroupVersion.String(),
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":            name,
			"namespace":       testNamespace,
			"resourceVersion": "42",
			"uid":             "aaaa",
		},
		"status": map[string]interface{}{"phase": "Ready"},
	}}
}

func newCompiler(t *testing.T, manifests NodeManifestFetcher, objs ...apiruntime.Object) *Compiler {
	t.Helper()
	scheme := apiruntime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("add v1alpha2 to scheme: %v", err)
	}
	return NewCompiler(fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build(), manifests)
}

func vmSnapshot(children ...string) *v1alpha2.VirtualMachineSnapshot {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			BoundSnapshotContentName: vmsContent,
		},
	}
	vms.Status.Conditions = readyConditions()
	for _, child := range children {
		vms.Status.ChildrenSnapshotRefs = append(vms.Status.ChildrenSnapshotRefs, v1alpha2.UnifiedSnapshotterChildRef{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskSnapshotKind,
			Name:       child,
		})
	}
	return vms
}

func vdSnapshot(ready bool, content string) *v1alpha2.VirtualDiskSnapshot {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds", Namespace: testNamespace},
		Status: v1alpha2.VirtualDiskSnapshotStatus{
			BoundSnapshotContentName: content,
		},
	}
	if ready {
		vds.Status.Conditions = readyConditions()
	}
	return vds
}

func TestCompileVirtualMachineSnapshot_PostOrderAndTransform(t *testing.T) {
	manifests := &fakeManifests{byContent: map[string][]unstructured.Unstructured{
		vmsContent: {obj(v1alpha2.VirtualMachineKind, "vm")},
		vdsContent: {obj(v1alpha2.VirtualDiskKind, "disk")},
	}}
	c := newCompiler(t, manifests,
		vmSnapshot("vds"),
		vdSnapshot(true, vdsContent),
	)

	objs, err := c.CompileVirtualMachineSnapshot(context.Background(), testNamespace, "vms")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got, want := len(objs), 2; got != want {
		t.Fatalf("compiled %d objects, want %d", got, want)
	}
	// Post-order: the disk (child) must precede the virtual machine (parent), so a sequential apply
	// creates what the VirtualMachine references before the VirtualMachine itself.
	if objs[0].GetKind() != v1alpha2.VirtualDiskKind || objs[1].GetKind() != v1alpha2.VirtualMachineKind {
		t.Fatalf("wrong order: %s then %s", objs[0].GetKind(), objs[1].GetKind())
	}
	// The nodes are addressed by their bound SnapshotContent, not by the snapshot CR name.
	if len(manifests.fetched) != 2 || manifests.fetched[0] != vdsContent || manifests.fetched[1] != vmsContent {
		t.Fatalf("fetched %v, want [%s %s]", manifests.fetched, vdsContent, vmsContent)
	}
	// Sanitized: no status / runtime metadata survives into apply-ready output.
	if _, found, _ := unstructured.NestedMap(objs[0].Object, "status"); found {
		t.Error("status survived sanitization")
	}
	if objs[0].GetResourceVersion() != "" || objs[0].GetUID() != "" {
		t.Error("runtime metadata survived sanitization")
	}
	// The restored disk provisions from its own VirtualDiskSnapshot.
	kind, _, _ := unstructured.NestedString(objs[0].Object, "spec", "dataSource", "objectRef", "kind")
	name, _, _ := unstructured.NestedString(objs[0].Object, "spec", "dataSource", "objectRef", "name")
	if kind != string(v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot) || name != "vds" {
		t.Errorf("dataSource.objectRef = %s/%s, want VirtualDiskSnapshot/vds", kind, name)
	}
	// An omitted targetNamespace means the snapshot's own namespace.
	if objs[0].GetNamespace() != testNamespace {
		t.Errorf("namespace = %q, want %q", objs[0].GetNamespace(), testNamespace)
	}
}

func TestCompileVirtualMachineSnapshot_FailsClosedOnNotReadyChild(t *testing.T) {
	manifests := &fakeManifests{byContent: map[string][]unstructured.Unstructured{
		vmsContent: {obj(v1alpha2.VirtualMachineKind, "vm")},
		vdsContent: {obj(v1alpha2.VirtualDiskKind, "disk")},
	}}
	c := newCompiler(t, manifests,
		vmSnapshot("vds"),
		vdSnapshot(false, vdsContent),
	)

	if _, err := c.CompileVirtualMachineSnapshot(context.Background(), testNamespace, "vms"); !errors.Is(err, ErrSnapshotNotReady) {
		t.Fatalf("err = %v, want ErrSnapshotNotReady", err)
	}
}

// A Ready node whose content is not bound yet is a race on the core's binder, not a corruption, so it is
// reported as not-ready (retryable) rather than compiled against an empty content name.
func TestCompileVirtualDiskSnapshot_UnboundContentIsNotReady(t *testing.T) {
	c := newCompiler(t, &fakeManifests{}, vdSnapshot(true, ""))

	if _, err := c.CompileVirtualDiskSnapshot(context.Background(), testNamespace, "vds"); !errors.Is(err, ErrSnapshotNotReady) {
		t.Fatalf("err = %v, want ErrSnapshotNotReady", err)
	}
}

func TestCompileSubtree_UnsupportedResource(t *testing.T) {
	c := newCompiler(t, &fakeManifests{})

	if _, err := c.CompileSubtree(context.Background(), "virtualmachines", testNamespace, "vm"); err == nil {
		t.Fatal("expected an error for a resource that is not a snapshot kind")
	}
}
