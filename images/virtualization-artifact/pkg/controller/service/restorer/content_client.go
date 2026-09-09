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

package restorer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// contentSubresourcesGroupVersion is state-snapshotter's own aggregated APIService group/version that
// hosts the cluster-scoped SnapshotContent's manifests-download subresource
// (github.com/deckhouse/state-snapshotter, images/state-snapshotter-controller/internal/api/restore_handler.go).
var contentSubresourcesGroupVersion = schema.GroupVersion{Group: "subresources.state-snapshotter.deckhouse.io", Version: "v1alpha1"}

// ContentClient downloads the raw manifests captured under a cluster-scoped SnapshotContent, via
// state-snapshotter's own generic manifests-download subresource — the mechanism its DataImport uses
// internally to read a captured object's original manifest
// (per_cr_manifests.go:BuildSingleNodeJSONFromContent). It goes through the normal kube-apiserver
// aggregation layer (in-cluster ServiceAccount token, no bespoke mTLS/CA) — the same principle already
// used in this codebase for the KubeVirt freeze/unfreeze subresource client, see api/client/kubeclient.
type ContentClient struct {
	restClient rest.Interface
}

func NewContentClient(cfg *rest.Config) (*ContentClient, error) {
	shallowCopy := *cfg
	shallowCopy.GroupVersion = &contentSubresourcesGroupVersion
	shallowCopy.APIPath = "/apis"
	shallowCopy.ContentType = runtime.ContentTypeJSON
	shallowCopy.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	restClient, err := rest.RESTClientFor(&shallowCopy)
	if err != nil {
		return nil, fmt.Errorf("build state-snapshotter content REST client: %w", err)
	}
	return &ContentClient{restClient: restClient}, nil
}

var (
	contentClientMu sync.RWMutex
	contentClient   *ContentClient
)

func InitContentClient(cfg *rest.Config) error {
	cc, err := NewContentClient(cfg)
	if err != nil {
		return err
	}

	contentClientMu.Lock()
	defer contentClientMu.Unlock()
	contentClient = cc
	return nil
}

// sharedContentClient returns the client InitContentClient installed.
func sharedContentClient() (*ContentClient, error) {
	contentClientMu.RLock()
	defer contentClientMu.RUnlock()

	if contentClient == nil {
		return nil, errors.New("state-snapshotter content client is not initialized: call restorer.InitContentClient at startup")
	}
	return contentClient, nil
}

// RawManifest is one object out of a manifests-download response: its own TypeMeta (used to demux by
// apiVersion+kind) plus the untouched raw bytes, decoded lazily by the caller into whichever concrete
// type matches.
type RawManifest struct {
	metav1.TypeMeta
	Raw json.RawMessage
}

func (m RawManifest) is(apiVersion, kind string) bool {
	return m.APIVersion == apiVersion && m.Kind == kind
}

// DownloadManifests fetches the flat, verbatim manifest set captured under the given cluster-scoped
// SnapshotContent — own node only, no subtree walk (a VirtualMachineSnapshot's own manifests, or a
// VirtualDiskSnapshot's; never both from one call).
func (c *ContentClient) DownloadManifests(ctx context.Context, snapshotContentName string) ([]RawManifest, error) {
	data, err := c.restClient.Get().
		Resource("snapshotcontents").
		Name(snapshotContentName).
		SubResource("manifests-download").
		DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("download manifests for SnapshotContent %q: %w", snapshotContentName, err)
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal manifests-download response for SnapshotContent %q: %w", snapshotContentName, err)
	}

	manifests := make([]RawManifest, 0, len(raw))
	for _, r := range raw {
		var tm metav1.TypeMeta
		if err := json.Unmarshal(r, &tm); err != nil {
			return nil, fmt.Errorf("unmarshal manifest TypeMeta for SnapshotContent %q: %w", snapshotContentName, err)
		}
		manifests = append(manifests, RawManifest{TypeMeta: tm, Raw: r})
	}
	return manifests, nil
}
