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
	"encoding/json"
	"fmt"
	"net/http"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/restore"
	"github.com/deckhouse/virtualization/api/subresources"
)

type ManifestsWithDataRestorationREST struct {
	resource string
	compiler *restore.Compiler
}

var (
	_ rest.Storage   = &ManifestsWithDataRestorationREST{}
	_ rest.Connecter = &ManifestsWithDataRestorationREST{}
)

func NewManifestsWithDataRestorationREST(resource string, compiler *restore.Compiler) *ManifestsWithDataRestorationREST {
	return &ManifestsWithDataRestorationREST{resource: resource, compiler: compiler}
}

func (r *ManifestsWithDataRestorationREST) New() runtime.Object {
	return &subresources.SnapshotManifestsWithDataRestoration{}
}

func (r *ManifestsWithDataRestorationREST) Destroy() {}

func (r *ManifestsWithDataRestorationREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.SnapshotManifestsWithDataRestoration{}, false, ""
}

func (r *ManifestsWithDataRestorationREST) ConnectMethods() []string {
	return []string{http.MethodGet}
}

func (r *ManifestsWithDataRestorationREST) Connect(ctx context.Context, name string, opts runtime.Object, _ rest.Responder) (http.Handler, error) {
	options, ok := opts.(*subresources.SnapshotManifestsWithDataRestoration)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}

	namespace := genericreq.NamespaceValue(ctx)
	if err := rejectForeignTargetNamespace(options.TargetNamespace, namespace); err != nil {
		return nil, k8serrors.NewBadRequest(err.Error())
	}

	objs, err := r.compiler.CompileSubtree(ctx, r.resource, namespace, name)
	if err != nil {
		return nil, apiError(r.resource, name, err)
	}
	if objs == nil {
		// Never emit "null": the core unmarshals the body into a slice.
		objs = []unstructured.Unstructured{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(objs)
	}), nil
}

// rejectForeignTargetNamespace refuses a request naming a target namespace other than the one the
// addressed snapshot lives in.
func rejectForeignTargetNamespace(requested []string, snapshotNamespace string) error {
	for _, ns := range requested {
		if ns == "" || ns == snapshotNamespace {
			continue
		}
		return fmt.Errorf("restoring into another namespace is not supported: snapshot namespace %q, requested %q", snapshotNamespace, ns)
	}
	return nil
}
