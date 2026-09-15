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

package rest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/nodeapi"
	"github.com/deckhouse/virtualization/api/subresources"
)

// ManifestsAndChildrenRefsUploadREST serves the manifests-and-children-refs-upload subresource: the way
// back in for a snapshot that was taken out of the cluster.
//
// `d8 snapshot restore` from an archive first creates one import-mode object per node of the tree, then
// posts each node's own manifests and its direct child refs here. Only an object with spec.mode: Import
// accepts this, so a capture's manifests can never be replaced by a request body; and the node has to be
// bound to a SnapshotContent first, which is why an unbound node is answered with a distinguishable
// "come back in a moment" rather than a failure.
type ManifestsAndChildrenRefsUploadREST struct {
	resource string
	service  *nodeapi.Service
}

var (
	_ rest.Storage   = &ManifestsAndChildrenRefsUploadREST{}
	_ rest.Connecter = &ManifestsAndChildrenRefsUploadREST{}
)

func NewManifestsAndChildrenRefsUploadREST(resource string, service *nodeapi.Service) *ManifestsAndChildrenRefsUploadREST {
	return &ManifestsAndChildrenRefsUploadREST{resource: resource, service: service}
}

func (r *ManifestsAndChildrenRefsUploadREST) New() runtime.Object {
	return &subresources.SnapshotManifestsAndChildrenRefsUpload{}
}

func (r *ManifestsAndChildrenRefsUploadREST) Destroy() {}

func (r *ManifestsAndChildrenRefsUploadREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.SnapshotManifestsAndChildrenRefsUpload{}, false, ""
}

func (r *ManifestsAndChildrenRefsUploadREST) ConnectMethods() []string {
	return []string{http.MethodPost}
}

// Connect hands back a handler rather than doing the work itself, because the work needs the request
// body — and because the answer needs a status reason of its own (see writeStatus).
func (r *ManifestsAndChildrenRefsUploadREST) Connect(ctx context.Context, name string, opts runtime.Object, _ rest.Responder) (http.Handler, error) {
	if _, ok := opts.(*subresources.SnapshotManifestsAndChildrenRefsUpload); !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}

	namespace := genericreq.NamespaceValue(ctx)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, ok := readUploadBody(w, req)
		if !ok {
			return
		}

		code, respBody, err := r.service.Upload(req.Context(), r.resource, namespace, name, body)
		if err != nil {
			var serviceErr *nodeapi.StatusError
			if errors.As(err, &serviceErr) {
				writeStatus(w, serviceErr)
				return
			}
			writeStatus(w, &nodeapi.StatusError{
				Code:    http.StatusInternalServerError,
				Reason:  "InternalError",
				Message: err.Error(),
			})
			return
		}

		// The core content layer answered with a Status of its own, for success as much as for failure.
		// It is relayed byte for byte: re-deriving one here would let this endpoint and the core describe
		// the same outcome differently, and the client has only one of the two to go on.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(respBody)
	}), nil
}

// readUploadBody reads the request body under a byte cap, answering the client itself and reporting
// ok=false when it cannot. The cap is what keeps one oversized request from being charged to the
// apiserver's memory: a node uploads only its own manifests, so a body past the limit is a client that
// has bundled something it should not have.
func readUploadBody(w http.ResponseWriter, req *http.Request) ([]byte, bool) {
	req.Body = http.MaxBytesReader(w, req.Body, nodeapi.MaxUploadBytes)

	body, err := io.ReadAll(req.Body)
	if err == nil {
		return body, true
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeStatus(w, &nodeapi.StatusError{
			Code:   http.StatusRequestEntityTooLarge,
			Reason: "RequestEntityTooLarge",
			Message: fmt.Sprintf("the upload payload is larger than the %d bytes this subresource accepts",
				nodeapi.MaxUploadBytes),
		})
		return nil, false
	}

	writeStatus(w, &nodeapi.StatusError{
		Code:    http.StatusBadRequest,
		Reason:  "BadRequest",
		Message: fmt.Sprintf("reading the upload payload failed: %v", err),
	})
	return nil, false
}
