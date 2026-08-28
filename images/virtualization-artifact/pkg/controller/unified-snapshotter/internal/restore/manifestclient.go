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

// Package restore compiles restore-ready manifests for an annotated VirtualMachineSnapshot subtree,
// without a dedicated aggregated apiserver of our own: it fetches each node's own raw manifest from the
// state-snapshotter core's already-existing, kind-agnostic manifests-download subresource (an external
// runtime dependency, deployed separately — see the unified-snapshotter ADRs), then sanitizes and
// transforms it in-process. See compile.go for the recursive per-node walk.
package restore

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// coreSubresourceGroupVersion is state-snapshotter core's aggregated subresources API group/version.
var coreSubresourceGroupVersion = schema.GroupVersion{
	Group:   "subresources.state-snapshotter.deckhouse.io",
	Version: "v1alpha1",
}

// ManifestClient fetches a single snapshot node's own (single-node, not-subtree) base manifest from the
// state-snapshotter core aggregated API. The result is raw (namespace-relative, with status) — the
// caller sanitizes it for restore.
type ManifestClient struct {
	rc rest.Interface
}

// NewManifestClient builds a ManifestClient from an in-cluster rest.Config.
func NewManifestClient(cfg *rest.Config) (*ManifestClient, error) {
	cfgCopy := rest.CopyConfig(cfg)
	cfgCopy.APIPath = "/apis"
	gv := coreSubresourceGroupVersion
	cfgCopy.GroupVersion = &gv
	cfgCopy.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	rc, err := rest.RESTClientFor(cfgCopy)
	if err != nil {
		return nil, fmt.Errorf("build state-snapshotter core manifests-download REST client: %w", err)
	}
	return &ManifestClient{rc: rc}, nil
}

// NodeBaseManifests fetches
// GET /apis/subresources.state-snapshotter.deckhouse.io/v1alpha1/snapshotcontents/<boundSnapshotContentName>/manifests-download
// from the core aggregated apiserver and decodes the returned JSON array of objects.
// boundSnapshotContentName is the node's own status.boundSnapshotContentName (a cluster-scoped
// SnapshotContent).
func (c *ManifestClient) NodeBaseManifests(ctx context.Context, boundSnapshotContentName string) ([]unstructured.Unstructured, error) {
	raw, err := c.rc.Get().
		AbsPath(
			"/apis", coreSubresourceGroupVersion.Group, coreSubresourceGroupVersion.Version,
			"snapshotcontents", boundSnapshotContentName, "manifests-download",
		).
		DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch node base manifests from state-snapshotter core (snapshotcontents/%s): %w", boundSnapshotContentName, err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var list []unstructured.Unstructured
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decode base manifests array: %w", err)
	}
	return list, nil
}
