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

// Package nodeapi implements the two per-CR aggregated subresources that carry a snapshot out of the
// cluster and back in: manifests-download and manifests-and-children-refs-upload.
//
// Both are addressed on one of our own snapshot objects and both reach the same place — the core's
// cluster-scoped SnapshotContent — through the content name that object records. Neither invents any
// content of its own: download relays the core's bytes verbatim, and upload relays the core's answer
// verbatim. What this package adds on top is the part the content layer cannot do, because a
// cluster-scoped request has no addressed CR to check anything against: it proves the content belongs to
// the snapshot named in the URL (see package node), and, on the way in, it enforces that the snapshot is
// an import target and records the children the payload declares on the snapshot's own status.
package nodeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/node"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/statuspatch"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// ReasonImportContentNotBound is the status.reason of the 409 an upload gets while the addressed
// snapshot has no bound SnapshotContent yet.
//
// The binder creates and binds contents from the import markers on its own schedule, independently of
// any upload, so this is a "come back in a moment", not a refusal — and the client tells the two apart
// by this reason alone, since both are 409. The string is the wire contract `d8 snapshot` retries on
// (aggapi.ReasonImportContentNotBound) and is shared verbatim with the core and with every other domain;
// it is spelled out here rather than imported because each module declares its own copy.
const ReasonImportContentNotBound = "ImportContentNotBound"

// MaxUploadBytes bounds one upload request body at 64 MiB. A node uploads its OWN manifests, never a
// subtree, so the whole archive is never in flight at once; the cap matches what `d8 snapshot` accepts in
// the other direction (aggapi.DefaultMaxResponseBytes).
const MaxUploadBytes int64 = 64 * 1024 * 1024

// ContentClient is the transport to the core's cluster-scoped SnapshotContent subresources, satisfied by
// *content.Client.
type ContentClient interface {
	// DownloadManifestsRaw returns the node's own captured manifests as the core encoded them.
	DownloadManifestsRaw(ctx context.Context, contentName string) ([]byte, error)
	// UploadManifests writes one node's own manifests and returns the core's status code and body
	// verbatim, reserving err for a transport failure that produced no response at all.
	UploadManifests(ctx context.Context, contentName string, manifests json.RawMessage) (int, []byte, error)
}

// StatusError is a rejection this package raises itself, carrying the HTTP status and the Kubernetes
// status reason the endpoint must answer with. Nothing the core answered is ever wrapped in it: those
// responses are relayed as they arrived.
type StatusError struct {
	Code    int
	Reason  string
	Message string
}

func (e *StatusError) Error() string { return e.Message }

func newStatusError(code int, reason, message string) *StatusError {
	return &StatusError{Code: code, Reason: reason, Message: message}
}

// Upload is the request body of manifests-and-children-refs-upload: one node's own manifests, plus the
// direct children it declares. The two halves are stored in different places, which is the whole shape
// of the endpoint — manifests go to the cluster-scoped content, children onto this node's own status.
type Upload struct {
	// Manifests is a JSON array of raw objects, in the shape manifests-download returns. It is forwarded
	// to the content layer byte for byte.
	Manifests json.RawMessage `json:"manifests"`
	// ChildRefs names the node's direct children. Their namespace is implicit — a run tree never leaves
	// the namespace it was captured in — and they are refs, not objects: each child is uploaded by its own
	// request against its own URL.
	ChildRefs []UploadChildRef `json:"childRefs"`
}

// UploadChildRef is one direct-child reference of an uploaded node.
type UploadChildRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

// Service serves both subresources. Reader must be uncached, for the reasons node.Load gives; Writer
// records children on the addressed snapshot's status.
type Service struct {
	Reader client.Reader
	Writer client.Client
	Core   ContentClient
}

func NewService(reader client.Reader, writer client.Client, core ContentClient) *Service {
	return &Service{Reader: reader, Writer: writer, Core: core}
}

// Download serves manifests-download: it resolves the addressed snapshot's SnapshotContent, proving the
// content belongs to it, and returns the core's bytes unchanged.
//
// There is no readiness gate here, unlike restore. Download answers what was captured, and a node that
// is mid-recapture or failed outright is exactly the node an operator needs to read; restore compiles
// something meant to be applied, which is why only restore refuses an unfinished node.
func (s *Service) Download(ctx context.Context, resource, namespace, name string) ([]byte, error) {
	n, err := node.Load(ctx, s.Reader, resource, namespace, name)
	if err != nil {
		return nil, err
	}
	contentName, err := node.ResolveBoundContent(ctx, s.Reader, n)
	if err != nil {
		return nil, err
	}
	return s.Core.DownloadManifestsRaw(ctx, contentName)
}

// Upload serves manifests-and-children-refs-upload. It returns the core content layer's status code and
// body to relay verbatim, or a *StatusError for anything it refuses before forwarding.
//
// The order of the steps is the contract, not a preference:
//
//  1. the payload is validated whole, so a bad child ref cannot leave half an upload applied;
//  2. spec.mode must be Import — capture-mode manifests describe a capture that really happened, and a
//     request body must never be able to overwrite them;
//  3. children are recorded on this node's own status BEFORE the bind gate, because they are a fact
//     about the snapshot tree rather than about the content: recording them early lets the tree be
//     walked as soon as the binder gets there, and it costs nothing to repeat;
//  4. the node must be bound, else the bind-first 409 the client waits on;
//  5. the content must point back at this node, else a forbidden that no retry can fix;
//  6. only the manifests are forwarded, and only then.
func (s *Service) Upload(ctx context.Context, resource, namespace, name string, body []byte) (int, []byte, error) {
	var payload Upload
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, nil, newStatusError(http.StatusBadRequest, "BadRequest", fmt.Sprintf("the request body is not a valid upload payload: %v", err))
	}

	manifests, err := validateManifests(payload.Manifests)
	if err != nil {
		return 0, nil, err
	}
	children, err := validateChildRefs(payload.ChildRefs)
	if err != nil {
		return 0, nil, err
	}

	n, err := node.Load(ctx, s.Reader, resource, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return 0, nil, newStatusError(http.StatusNotFound, "NotFound", fmt.Sprintf("%s %s/%s not found", resource, namespace, name))
		}
		return 0, nil, newStatusError(http.StatusInternalServerError, "InternalError", err.Error())
	}

	if !n.IsImport() {
		return 0, nil, newStatusError(http.StatusConflict, "Conflict", fmt.Sprintf(
			"%s %s/%s does not have spec.mode: Import, so its manifests come from a capture and cannot be uploaded",
			resource, namespace, name))
	}

	if err := s.recordChildren(ctx, resource, namespace, name, children); err != nil {
		return 0, nil, err
	}

	if n.ContentName == "" {
		return 0, nil, newStatusError(http.StatusConflict, ReasonImportContentNotBound, fmt.Sprintf(
			"%s %s/%s is not bound to a SnapshotContent yet: waiting for the binder", resource, namespace, name))
	}

	contentName, err := node.ResolveBoundContent(ctx, s.Reader, n)
	if err != nil {
		return 0, nil, asStatusError(err)
	}

	code, respBody, err := s.Core.UploadManifests(ctx, contentName, manifests)
	if err != nil {
		// No response was obtained from the core, so there is nothing to relay and the failure is
		// upstream of us rather than in us: 502, with a reason that matches the code. "InternalError"
		// would map to 500 and point a client at the wrong server.
		return 0, nil, newStatusError(http.StatusBadGateway, "BadGateway", fmt.Sprintf(
			"forwarding the manifests to SnapshotContent %q failed: %v", contentName, err))
	}
	return code, respBody, nil
}

// recordChildren writes status.childrenSnapshotRefs on the addressed snapshot from the uploaded child
// list.
//
// It is a merge patch naming that one field, which is what makes it safe to run alongside the object's
// own controller: the phase that controller maintains is never mentioned, so it is never overwritten, and
// there is no read-modify-write window for a concurrent status write to be lost in. Repeating the same
// upload therefore costs one idempotent patch rather than a conflict to retry.
func (s *Service) recordChildren(ctx context.Context, resource, namespace, name string, children []v1alpha2.UnifiedSnapshotterChildRef) error {
	patch, err := childrenPatch(resource, children)
	if err != nil {
		return newStatusError(http.StatusInternalServerError, "InternalError", err.Error())
	}

	obj, err := node.EmptyObject(resource)
	if err != nil {
		return newStatusError(http.StatusInternalServerError, "InternalError", err.Error())
	}
	obj.SetNamespace(namespace)
	obj.SetName(name)

	err = s.Writer.Status().Patch(ctx, obj, patch)
	switch {
	case err == nil:
		return nil
	case apierrors.IsNotFound(err):
		return newStatusError(http.StatusNotFound, "NotFound", fmt.Sprintf("%s %s/%s not found", resource, namespace, name))
	case apierrors.IsConflict(err):
		// A merge patch takes no optimistic lock, so this is not a lost-update race but something
		// upstream refusing the write — an admission webhook, say. Relayed as a 409 the client may retry.
		return newStatusError(http.StatusConflict, "Conflict", fmt.Sprintf(
			"recording the uploaded children on %s %s/%s was refused as conflicting: %v", resource, namespace, name, err))
	default:
		return newStatusError(http.StatusInternalServerError, "InternalError", fmt.Sprintf(
			"recording the uploaded children on %s %s/%s failed: %v", resource, namespace, name, err))
	}
}

// childrenPatch builds the merge patch that sets status.childrenSnapshotRefs and nothing else. An empty
// list is written as an empty array rather than omitted, so re-uploading a node that lost its children
// clears them instead of leaving the previous set in place.
func childrenPatch(resource string, children []v1alpha2.UnifiedSnapshotterChildRef) (client.Patch, error) {
	kind, ok := node.KindForResource(resource)
	if !ok {
		return nil, fmt.Errorf("unsupported snapshot resource %q", resource)
	}
	return statuspatch.For(v1alpha2.SchemeGroupVersion.WithKind(kind), map[string]any{
		"childrenSnapshotRefs": children,
	})
}

// validateManifests requires the manifests key to be present and to be an array, and returns the
// original bytes rather than a re-encoding: what the core stores must be what the client sent.
func validateManifests(manifests json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(manifests)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, newStatusError(http.StatusBadRequest, "BadRequest", "the upload payload has no manifests")
	}
	var array []json.RawMessage
	if err := json.Unmarshal(trimmed, &array); err != nil {
		return nil, newStatusError(http.StatusBadRequest, "BadRequest", fmt.Sprintf("the upload payload's manifests must be a JSON array: %v", err))
	}
	return manifests, nil
}

// validateChildRefs rejects an incomplete ref, and any child outside our own snapshot kinds. The second
// half is what keeps the recorded tree walkable: every reader of status.childrenSnapshotRefs — our
// restore recursion included — resolves a child through node.ResourceForKind, and a ref it cannot
// resolve is a subtree nothing can read back.
func validateChildRefs(refs []UploadChildRef) ([]v1alpha2.UnifiedSnapshotterChildRef, error) {
	out := make([]v1alpha2.UnifiedSnapshotterChildRef, 0, len(refs))
	for i, ref := range refs {
		if ref.APIVersion == "" || ref.Kind == "" || ref.Name == "" {
			return nil, newStatusError(http.StatusBadRequest, "BadRequest", fmt.Sprintf(
				"childRefs[%d] must set apiVersion, kind and name", i))
		}
		if _, ok := node.ResourceForKind(ref.Kind); !ok || ref.APIVersion != v1alpha2.SchemeGroupVersion.String() {
			return nil, newStatusError(http.StatusBadRequest, "BadRequest", fmt.Sprintf(
				"childRefs[%d] is a %s %s, which is not a snapshot kind of %s: a node may only declare children of its own domain",
				i, ref.APIVersion, ref.Kind, v1alpha2.SchemeGroupVersion.String()))
		}
		out = append(out, v1alpha2.UnifiedSnapshotterChildRef{APIVersion: ref.APIVersion, Kind: ref.Kind, Name: ref.Name})
	}
	return out, nil
}

// asStatusError translates a Kubernetes error raised while resolving the bound content into the response
// this endpoint should give, keeping the code and the reason consistent with each other.
func asStatusError(err error) error {
	switch {
	case apierrors.IsNotFound(err):
		return newStatusError(http.StatusNotFound, "NotFound", err.Error())
	case apierrors.IsForbidden(err):
		return newStatusError(http.StatusForbidden, "Forbidden", err.Error())
	default:
		return newStatusError(http.StatusInternalServerError, "InternalError", err.Error())
	}
}
