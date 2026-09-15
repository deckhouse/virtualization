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

package nodeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

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
	contentName   = "content-vms"
)

// fakeCore records what reached the core content layer and answers with a canned response.
type fakeCore struct {
	download      []byte
	downloadErr   error
	uploadCode    int
	uploadBody    []byte
	uploadErr     error
	uploadedTo    string
	uploadedBytes json.RawMessage
	uploads       int
}

func (f *fakeCore) DownloadManifestsRaw(_ context.Context, name string) ([]byte, error) {
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	if name != contentName {
		return nil, fmt.Errorf("unexpected content %q", name)
	}
	return f.download, nil
}

func (f *fakeCore) UploadManifests(_ context.Context, name string, manifests json.RawMessage) (int, []byte, error) {
	f.uploads++
	f.uploadedTo = name
	f.uploadedBytes = manifests
	if f.uploadErr != nil {
		return 0, nil, f.uploadErr
	}
	return f.uploadCode, f.uploadBody, nil
}

func scheme(t *testing.T) *apiruntime.Scheme {
	t.Helper()
	s := apiruntime.NewScheme()
	if err := v1alpha2.AddToScheme(s); err != nil {
		t.Fatalf("add v1alpha2 to scheme: %v", err)
	}
	if err := storagev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add state-snapshotter storage/v1alpha1 to scheme: %v", err)
	}
	return s
}

type snapshotOptions struct {
	mode    v1alpha2.UnifiedSnapshotterMode
	content string
	// backRefTo, when set, is the snapshot name the SnapshotContent points back at. Empty means the
	// snapshot's own name, which is the shape the core's binder produces.
	backRefTo string
	// noContent suppresses the SnapshotContent entirely, modelling a node whose binder has not run.
	noContent bool
}

func newService(t *testing.T, core ContentClient, opts snapshotOptions) *Service {
	t.Helper()

	vms := &v1alpha2.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vms", Namespace: testNamespace, UID: types.UID("vms-uid")},
		Spec:       v1alpha2.VirtualMachineSnapshotSpec{Mode: opts.mode},
		Status:     v1alpha2.VirtualMachineSnapshotStatus{BoundSnapshotContentName: opts.content},
	}

	objs := []client.Object{vms}
	if opts.content != "" && !opts.noContent {
		backRefTo := opts.backRefTo
		if backRefTo == "" {
			backRefTo = vms.Name
		}
		objs = append(objs, &storagev1alpha1.SnapshotContent{
			ObjectMeta: metav1.ObjectMeta{Name: opts.content},
			Spec: storagev1alpha1.SnapshotContentSpec{
				SnapshotRef: &storagev1alpha1.SnapshotSubjectRef{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.VirtualMachineSnapshotKind,
					Namespace:  testNamespace,
					Name:       backRefTo,
				},
			},
		})
	}

	cli := fake.NewClientBuilder().
		WithScheme(scheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha2.VirtualMachineSnapshot{}, &v1alpha2.VirtualDiskSnapshot{}).
		Build()

	return NewService(cli, cli, core)
}

func childrenOf(t *testing.T, s *Service) []v1alpha2.UnifiedSnapshotterChildRef {
	t.Helper()
	vms := &v1alpha2.VirtualMachineSnapshot{}
	key := types.NamespacedName{Namespace: testNamespace, Name: "vms"}
	if err := s.Reader.Get(context.Background(), key, vms); err != nil {
		t.Fatalf("re-read the snapshot: %v", err)
	}
	return vms.Status.ChildrenSnapshotRefs
}

func statusError(t *testing.T, err error) *StatusError {
	t.Helper()
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v (%T), want a *StatusError", err, err)
	}
	return se
}

func uploadBody(t *testing.T, manifests string, children ...UploadChildRef) []byte {
	t.Helper()
	body, err := json.Marshal(Upload{Manifests: json.RawMessage(manifests), ChildRefs: children})
	if err != nil {
		t.Fatalf("marshal upload body: %v", err)
	}
	return body
}

func vdsChild(name string) UploadChildRef {
	return UploadChildRef{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualDiskSnapshotKind,
		Name:       name,
	}
}

// Download is a proxy: the core's bytes reach the client unchanged, because an archive has to record
// what was captured and a re-encoding here would reorder keys and drop what our types do not model.
func TestDownload_RelaysTheCoreBytesVerbatim(t *testing.T) {
	raw := []byte(`[{"kind":"VirtualMachine","apiVersion":"virtualization.deckhouse.io/v1alpha2","zzz":1}]`)
	core := &fakeCore{download: raw}
	s := newService(t, core, snapshotOptions{content: contentName})

	got, err := s.Download(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("download returned %s, want the core's bytes %s", got, raw)
	}
}

// A capture-mode snapshot is downloadable: download reports what was captured, and readiness is a
// restore-only gate.
func TestDownload_DoesNotRequireImportMode(t *testing.T) {
	core := &fakeCore{download: []byte(`[]`)}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeCapture, content: contentName})

	if _, err := s.Download(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms"); err != nil {
		t.Fatalf("download: %v", err)
	}
}

func TestDownload_RefusesAContentThatDoesNotPointBack(t *testing.T) {
	core := &fakeCore{download: []byte(`[]`)}
	s := newService(t, core, snapshotOptions{content: contentName, backRefTo: "someone-else"})

	_, err := s.Download(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms")
	if err == nil {
		t.Fatal("download succeeded against a content bound to another snapshot")
	}
}

func TestDownload_RefusesAnUnboundNode(t *testing.T) {
	core := &fakeCore{download: []byte(`[]`)}
	s := newService(t, core, snapshotOptions{})

	if _, err := s.Download(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms"); err == nil {
		t.Fatal("download succeeded on a node with no bound SnapshotContent")
	}
}

// A capture's manifests describe a capture that really happened. Nothing in a request body may replace
// them, so a node that is not an import target is refused before anything is written or forwarded.
func TestUpload_RefusesACaptureModeNode(t *testing.T) {
	core := &fakeCore{uploadCode: http.StatusOK}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeCapture, content: contentName})

	_, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms",
		uploadBody(t, `[]`, vdsChild("child")))

	if got := statusError(t, err).Code; got != http.StatusConflict {
		t.Errorf("code = %d, want %d", got, http.StatusConflict)
	}
	if core.uploads != 0 {
		t.Error("manifests were forwarded for a capture-mode node")
	}
	if children := childrenOf(t, s); len(children) != 0 {
		t.Errorf("children were recorded for a capture-mode node: %+v", children)
	}
}

// The bind-first contract: an unbound node is refused with a reason of its own, because that is the only
// thing telling `d8 snapshot` to wait rather than to give up — both answers are 409.
func TestUpload_UnboundNodeIsRefusedWithItsOwnReason(t *testing.T) {
	core := &fakeCore{uploadCode: http.StatusOK}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport})

	_, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms",
		uploadBody(t, `[]`, vdsChild("child")))

	se := statusError(t, err)
	if se.Code != http.StatusConflict {
		t.Errorf("code = %d, want %d", se.Code, http.StatusConflict)
	}
	if se.Reason != ReasonImportContentNotBound {
		t.Errorf("reason = %q, want %q", se.Reason, ReasonImportContentNotBound)
	}
	if core.uploads != 0 {
		t.Error("manifests were forwarded before the node was bound")
	}
	// Children are a fact about the tree, not about the content, so they are recorded even here: the
	// tree becomes walkable as soon as the binder catches up.
	if children := childrenOf(t, s); len(children) != 1 || children[0].Name != "child" {
		t.Errorf("children = %+v, want the uploaded child recorded before the bind gate", children)
	}
}

func TestUpload_RecordsChildrenAndForwardsTheManifests(t *testing.T) {
	coreStatus := []byte(`{"kind":"Status","status":"Success","details":{"name":"mcp-1"}}`)
	core := &fakeCore{uploadCode: http.StatusCreated, uploadBody: coreStatus}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName})

	manifests := `[{"apiVersion":"v1","kind":"Secret","metadata":{"name":"s"}}]`
	code, body, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms",
		uploadBody(t, manifests, vdsChild("a"), vdsChild("b")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if code != http.StatusCreated {
		t.Errorf("code = %d, want the core's %d relayed", code, http.StatusCreated)
	}
	if string(body) != string(coreStatus) {
		t.Errorf("body = %s, want the core's status relayed verbatim", body)
	}
	if core.uploadedTo != contentName {
		t.Errorf("forwarded to %q, want %q", core.uploadedTo, contentName)
	}
	if string(core.uploadedBytes) != manifests {
		t.Errorf("forwarded %s, want the manifests unchanged %s", core.uploadedBytes, manifests)
	}

	children := childrenOf(t, s)
	if len(children) != 2 || children[0].Name != "a" || children[1].Name != "b" {
		t.Errorf("children = %+v, want both uploaded children recorded", children)
	}
	if children[0].Kind != v1alpha2.VirtualDiskSnapshotKind {
		t.Errorf("child kind = %q, want %q", children[0].Kind, v1alpha2.VirtualDiskSnapshotKind)
	}
}

// A repeated upload is the normal case — `d8 snapshot` retries — so it has to be harmless.
func TestUpload_IsRepeatable(t *testing.T) {
	core := &fakeCore{uploadCode: http.StatusOK, uploadBody: []byte(`{}`)}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName})
	body := uploadBody(t, `[]`, vdsChild("a"))

	for attempt := 1; attempt <= 2; attempt++ {
		if _, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms", body); err != nil {
			t.Fatalf("upload attempt %d: %v", attempt, err)
		}
	}
	if children := childrenOf(t, s); len(children) != 1 {
		t.Errorf("children = %+v, want the repeated upload to leave one child", children)
	}
}

func TestUpload_RefusesAContentThatDoesNotPointBack(t *testing.T) {
	core := &fakeCore{uploadCode: http.StatusOK}
	s := newService(t, core, snapshotOptions{
		mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName, backRefTo: "someone-else",
	})

	_, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms",
		uploadBody(t, `[]`))

	if got := statusError(t, err).Code; got != http.StatusForbidden {
		t.Errorf("code = %d, want %d", got, http.StatusForbidden)
	}
	if core.uploads != 0 {
		t.Error("manifests were forwarded into a content bound to another snapshot")
	}
}

func TestUpload_RejectsAMalformedPayload(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{name: "not JSON at all", body: []byte(`{`)},
		{name: "no manifests key", body: []byte(`{"childRefs":[]}`)},
		{name: "null manifests", body: []byte(`{"manifests":null}`)},
		{name: "manifests is an object", body: []byte(`{"manifests":{"kind":"Secret"}}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := &fakeCore{uploadCode: http.StatusOK}
			s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName})

			_, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms", tt.body)
			if got := statusError(t, err).Code; got != http.StatusBadRequest {
				t.Errorf("code = %d, want %d", got, http.StatusBadRequest)
			}
			if core.uploads != 0 {
				t.Error("a malformed payload reached the core")
			}
		})
	}
}

// A child ref nothing can resolve is a subtree nothing can read back, so it is refused before any write.
func TestUpload_RejectsAnUnresolvableChild(t *testing.T) {
	tests := []struct {
		name  string
		child UploadChildRef
	}{
		{name: "no name", child: UploadChildRef{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskSnapshotKind}},
		{name: "no kind", child: UploadChildRef{APIVersion: v1alpha2.SchemeGroupVersion.String(), Name: "a"}},
		{name: "a foreign kind", child: UploadChildRef{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: "VirtualDisk", Name: "a"}},
		{name: "a foreign group", child: UploadChildRef{APIVersion: "state-snapshotter.deckhouse.io/v1alpha1", Kind: "Snapshot", Name: "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := &fakeCore{uploadCode: http.StatusOK}
			s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName})

			_, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms",
				uploadBody(t, `[]`, tt.child))
			if got := statusError(t, err).Code; got != http.StatusBadRequest {
				t.Errorf("code = %d, want %d", got, http.StatusBadRequest)
			}
			if children := childrenOf(t, s); len(children) != 0 {
				t.Errorf("children = %+v, want nothing recorded from a rejected payload", children)
			}
		})
	}
}

// A leaf declares no children, and its upload must not be mistaken for a malformed one.
func TestUpload_AcceptsALeafWithNoChildren(t *testing.T) {
	core := &fakeCore{uploadCode: http.StatusOK, uploadBody: []byte(`{}`)}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName})

	if _, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms",
		uploadBody(t, `[]`)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if core.uploads != 1 {
		t.Errorf("forwarded %d times, want 1", core.uploads)
	}
}

// An HTTP error from the core is relayed, not translated: a second mapping of the same condition would
// drift from the core's, and the client only sees one of the two.
func TestUpload_RelaysACoreRejection(t *testing.T) {
	coreStatus := []byte(`{"kind":"Status","status":"Failure","reason":"BadRequest","code":400}`)
	core := &fakeCore{uploadCode: http.StatusBadRequest, uploadBody: coreStatus}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName})

	code, body, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms",
		uploadBody(t, `[]`))
	if err != nil {
		t.Fatalf("upload returned err = %v; a core rejection is relayed, not raised", err)
	}
	if code != http.StatusBadRequest {
		t.Errorf("code = %d, want the core's %d", code, http.StatusBadRequest)
	}
	if string(body) != string(coreStatus) {
		t.Errorf("body = %s, want the core's status verbatim", body)
	}
}

// Failing to reach the core at all is an upstream failure. It answers 502 rather than 500 so a client
// keying off the reason is not pointed at the wrong server.
func TestUpload_ReportsATransportFailureAsBadGateway(t *testing.T) {
	core := &fakeCore{uploadErr: errors.New("connection refused")}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName})

	_, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "vms",
		uploadBody(t, `[]`))

	se := statusError(t, err)
	if se.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want %d", se.Code, http.StatusBadGateway)
	}
	if se.Reason != "BadGateway" {
		t.Errorf("reason = %q, want BadGateway", se.Reason)
	}
}

func TestUpload_ReportsAMissingNodeAsNotFound(t *testing.T) {
	core := &fakeCore{uploadCode: http.StatusOK}
	s := newService(t, core, snapshotOptions{mode: v1alpha2.UnifiedSnapshotterModeImport, content: contentName})

	_, _, err := s.Upload(context.Background(), v1alpha2.VirtualMachineSnapshotResource, testNamespace, "gone",
		uploadBody(t, `[]`))
	if got := statusError(t, err).Code; got != http.StatusNotFound {
		t.Errorf("code = %d, want %d", got, http.StatusNotFound)
	}
}
