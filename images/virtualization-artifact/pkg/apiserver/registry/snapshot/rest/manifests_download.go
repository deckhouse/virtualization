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
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/nodeapi"
	"github.com/deckhouse/virtualization/api/subresources"
)

// ManifestsDownloadREST serves the manifests-download subresource of one snapshot object: the objects
// that node captured, as they were captured.
//
// It is what `d8 snapshot download` reads to put a node into an archive, so its output is deliberately
// unlike its restore-compiling sibling's: status is preserved and nothing is rewritten, because an
// archive has to be able to describe the cluster as it was, and the decisions about what a restore
// should look like are made when the archive is applied, against the cluster it is applied to.
type ManifestsDownloadREST struct {
	resource string
	service  *nodeapi.Service
}

var (
	_ rest.Storage   = &ManifestsDownloadREST{}
	_ rest.Connecter = &ManifestsDownloadREST{}
)

func NewManifestsDownloadREST(resource string, service *nodeapi.Service) *ManifestsDownloadREST {
	return &ManifestsDownloadREST{resource: resource, service: service}
}

func (r *ManifestsDownloadREST) New() runtime.Object {
	return &subresources.SnapshotManifestsDownload{}
}

func (r *ManifestsDownloadREST) Destroy() {}

func (r *ManifestsDownloadREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.SnapshotManifestsDownload{}, false, ""
}

func (r *ManifestsDownloadREST) ConnectMethods() []string {
	return []string{http.MethodGet}
}

func (r *ManifestsDownloadREST) Connect(ctx context.Context, name string, opts runtime.Object, _ rest.Responder) (http.Handler, error) {
	if _, ok := opts.(*subresources.SnapshotManifestsDownload); !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}

	manifests, err := r.service.Download(ctx, r.resource, genericreq.NamespaceValue(ctx), name)
	if err != nil {
		return nil, apiError(r.resource, name, err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(manifests)
	}), nil
}
