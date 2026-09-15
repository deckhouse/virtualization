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

package node

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	testNamespace = "ns"
	testUID       = types.UID("vms-uid")
)

func newReader(t *testing.T, objs ...client.Object) client.Reader {
	t.Helper()
	scheme := apiruntime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("add v1alpha2 to scheme: %v", err)
	}
	if err := storagev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add state-snapshotter storage/v1alpha1 to scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func testNode() Node {
	return Node{
		APIVersion:  v1alpha2.SchemeGroupVersion.String(),
		Kind:        v1alpha2.VirtualMachineSnapshotKind,
		Namespace:   testNamespace,
		Name:        "vms",
		UID:         testUID,
		ContentName: "content-1",
	}
}

func contentWith(ref *storagev1alpha1.SnapshotSubjectRef) *storagev1alpha1.SnapshotContent {
	return &storagev1alpha1.SnapshotContent{
		ObjectMeta: metav1.ObjectMeta{Name: "content-1"},
		Spec:       storagev1alpha1.SnapshotContentSpec{SnapshotRef: ref},
	}
}

func matchingRef() *storagev1alpha1.SnapshotSubjectRef {
	return &storagev1alpha1.SnapshotSubjectRef{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualMachineSnapshotKind,
		Namespace:  testNamespace,
		Name:       "vms",
		UID:        testUID,
	}
}

func TestLoad_ReadsBothSnapshotKinds(t *testing.T) {
	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: testNamespace, UID: testUID},
		Spec: v1alpha2.VirtualMachineSnapshotSpec{
			Mode:          v1alpha2.UnifiedSnapshotterModeImport,
			KeepIPAddress: v1alpha2.KeepIPAddressNever,
		},
		Status: v1alpha2.VirtualMachineSnapshotStatus{
			BoundSnapshotContentName: "content-1",
			Conditions: []metav1.Condition{{
				Type: v1alpha2.UnifiedSnapshotterConditionReady, Status: metav1.ConditionTrue, Reason: "Ok",
			}},
			ChildrenSnapshotRefs: []v1alpha2.UnifiedSnapshotterChildRef{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualDiskSnapshotKind,
				Name:       "vds",
			}},
		},
	}
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds", Namespace: testNamespace, UID: types.UID("vds-uid")},
		Status:     v1alpha2.VirtualDiskSnapshotStatus{BoundSnapshotContentName: "content-2"},
	}
	reader := newReader(t, vms, vds)

	got, err := Load(context.Background(), reader, v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms")
	if err != nil {
		t.Fatalf("load the machine snapshot: %v", err)
	}
	if !got.IsImport() {
		t.Error("the machine snapshot did not report import mode")
	}
	if !got.Ready || got.ContentName != "content-1" || got.UID != testUID {
		t.Errorf("machine snapshot = %+v, want it Ready, bound to content-1 and carrying its uid", got)
	}
	if len(got.Children) != 1 || got.Children[0].Name != "vds" {
		t.Errorf("children = %+v, want the declared child", got.Children)
	}
	if got.KeepIPAddress != v1alpha2.KeepIPAddressNever {
		t.Errorf("keepIPAddress = %q, want %q", got.KeepIPAddress, v1alpha2.KeepIPAddressNever)
	}

	gotDisk, err := Load(context.Background(), reader, v1alpha2.VirtualDiskSnapshotResource, testNamespace, "vds")
	if err != nil {
		t.Fatalf("load the disk snapshot: %v", err)
	}
	if gotDisk.IsImport() {
		t.Error("a snapshot with no spec.mode reported import mode")
	}
	if gotDisk.Ready {
		t.Error("a snapshot with no Ready condition reported Ready")
	}
	if gotDisk.Kind != v1alpha2.VirtualDiskSnapshotKind {
		t.Errorf("kind = %q, want %q", gotDisk.Kind, v1alpha2.VirtualDiskSnapshotKind)
	}
}

func TestLoad_RejectsAnUnknownResource(t *testing.T) {
	if _, err := Load(context.Background(), newReader(t), "virtualmachines", testNamespace, "vm"); err == nil {
		t.Fatal("load accepted a resource that is not one of this domain's snapshot kinds")
	}
}

func TestLoad_PropagatesNotFound(t *testing.T) {
	_, err := Load(context.Background(), newReader(t), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "gone")
	if !apierrors.IsNotFound(err) {
		t.Fatalf("err = %v, want a NotFound that callers can recognise through the wrapping", err)
	}
}

// status.boundSnapshotContentName is a status field on a namespaced object, so the name alone proves
// nothing. Every field of the content's own back-ref has to agree, or a tenant who can write that name
// could read — or write into — another snapshot's captured manifests.
func TestResolveBoundContent_RequiresAnExactBackRef(t *testing.T) {
	tests := []struct {
		name string
		ref  func() *storagev1alpha1.SnapshotSubjectRef
	}{
		{
			name: "no back-ref at all",
			ref:  func() *storagev1alpha1.SnapshotSubjectRef { return nil },
		},
		{
			name: "another apiVersion",
			ref: func() *storagev1alpha1.SnapshotSubjectRef {
				r := matchingRef()
				r.APIVersion = "state-snapshotter.deckhouse.io/v1alpha1"
				return r
			},
		},
		{
			name: "another kind",
			ref: func() *storagev1alpha1.SnapshotSubjectRef {
				r := matchingRef()
				r.Kind = v1alpha2.VirtualDiskSnapshotKind
				return r
			},
		},
		{
			name: "another namespace",
			ref: func() *storagev1alpha1.SnapshotSubjectRef {
				r := matchingRef()
				r.Namespace = "other"
				return r
			},
		},
		{
			name: "another name",
			ref: func() *storagev1alpha1.SnapshotSubjectRef {
				r := matchingRef()
				r.Name = "someone-else"
				return r
			},
		},
		{
			// A same-name snapshot recreated after the content was bound is a different object, and the
			// uid is the only thing that says so.
			name: "the same name at another uid",
			ref: func() *storagev1alpha1.SnapshotSubjectRef {
				r := matchingRef()
				r.UID = types.UID("recreated")
				return r
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newReader(t, contentWith(tt.ref()))

			_, err := ResolveBoundContent(context.Background(), reader, testNode())
			if !apierrors.IsForbidden(err) {
				t.Fatalf("err = %v, want Forbidden: a mismatch can never be fixed by retrying", err)
			}
		})
	}
}

func TestResolveBoundContent_AcceptsAMatchingBackRef(t *testing.T) {
	reader := newReader(t, contentWith(matchingRef()))

	got, err := ResolveBoundContent(context.Background(), reader, testNode())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "content-1" {
		t.Errorf("resolved %q, want content-1", got)
	}
}

// The core leaves the uid empty on paths that have none to record, and there the remaining four fields
// are the whole answer. Comparing against an empty uid would refuse a content the core itself bound.
func TestResolveBoundContent_AcceptsARefWithNoUID(t *testing.T) {
	ref := matchingRef()
	ref.UID = ""
	reader := newReader(t, contentWith(ref))

	if _, err := ResolveBoundContent(context.Background(), reader, testNode()); err != nil {
		t.Fatalf("resolve: %v", err)
	}
}

// An unbound node is a race with the binder, not a violation, so it answers NotFound rather than
// Forbidden: the caller may retry.
func TestResolveBoundContent_ReportsAnUnboundNodeAsNotFound(t *testing.T) {
	n := testNode()
	n.ContentName = ""

	_, err := ResolveBoundContent(context.Background(), newReader(t), n)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("err = %v, want NotFound", err)
	}
}

func TestResourceForKind(t *testing.T) {
	tests := []struct {
		kind     string
		resource string
		ok       bool
	}{
		{kind: v1alpha2.VirtualMachineSnapshotKind, resource: v1alpha2.VirtualMachineSnapshotResource, ok: true},
		{kind: v1alpha2.VirtualDiskSnapshotKind, resource: v1alpha2.VirtualDiskSnapshotResource, ok: true},
		{kind: v1alpha2.VirtualDiskKind},
		{kind: "Snapshot"},
		{kind: ""},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			resource, ok := ResourceForKind(tt.kind)
			if ok != tt.ok || resource != tt.resource {
				t.Errorf("ResourceForKind(%q) = (%q, %v), want (%q, %v)", tt.kind, resource, ok, tt.resource, tt.ok)
			}
			if !tt.ok {
				return
			}
			// The two maps have to stay each other's inverse: one is how a child ref is resolved, the
			// other how a status patch names the object it addresses.
			if kind, ok := KindForResource(resource); !ok || kind != tt.kind {
				t.Errorf("KindForResource(%q) = (%q, %v), want (%q, true)", resource, kind, ok, tt.kind)
			}
			if _, err := EmptyObject(resource); err != nil {
				t.Errorf("EmptyObject(%q): %v", resource, err)
			}
		})
	}
}
