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

// Package content is the single transport to the state-snapshotter core's cluster-scoped SnapshotContent
// subresources: manifests-download (read) and manifests-upload (write).
package content

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

var subresourcesGroupVersion = schema.GroupVersion{Group: "subresources.state-snapshotter.deckhouse.io", Version: "v1alpha1"}

type Client struct {
	restClient rest.Interface
}

func NewClient(cfg *rest.Config) (*Client, error) {
	shallowCopy := *cfg
	shallowCopy.GroupVersion = &subresourcesGroupVersion
	shallowCopy.APIPath = "/apis"
	shallowCopy.ContentType = runtime.ContentTypeJSON
	shallowCopy.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	restClient, err := rest.RESTClientFor(&shallowCopy)
	if err != nil {
		return nil, fmt.Errorf("build state-snapshotter content REST client: %w", err)
	}
	return &Client{restClient: restClient}, nil
}

// RawManifest is one object out of a manifests-download response.
type RawManifest struct {
	metav1.TypeMeta
	Raw json.RawMessage
}

func (m RawManifest) Is(apiVersion, kind string) bool {
	return m.APIVersion == apiVersion && m.Kind == kind
}

// DownloadManifestsRaw returns the response body of snapshotcontents/<name>/manifests-download exactly
// as the core sent it: a JSON array of the node's own captured objects, status preserved and namespace
// made relative. The manifests-download subresource we serve on our own snapshot kinds relays these
// bytes unchanged, so it must not re-encode them — a round trip through a decoder would reorder keys and
// drop anything our types do not model.
func (c *Client) DownloadManifestsRaw(ctx context.Context, snapshotContentName string) ([]byte, error) {
	data, err := c.restClient.Get().
		Resource("snapshotcontents").
		Name(snapshotContentName).
		SubResource("manifests-download").
		DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("download manifests for SnapshotContent %q: %w", snapshotContentName, err)
	}
	return data, nil
}

func (c *Client) DownloadManifests(ctx context.Context, snapshotContentName string) ([]RawManifest, error) {
	data, err := c.DownloadManifestsRaw(ctx, snapshotContentName)
	if err != nil {
		return nil, err
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

// manifestsUploadBody is the wire body of snapshotcontents/<name>/manifests-upload. The cluster-scoped
// content layer takes manifests only: childRefs are an attribute of the namespaced snapshot node and are
// recorded on that node's own status, never forwarded here.
type manifestsUploadBody struct {
	Manifests json.RawMessage `json:"manifests"`
}

// UploadManifests forwards one node's own manifests to the core's cluster-scoped
// snapshotcontents/<name>/manifests-upload and returns the core's HTTP status code together with its raw
// response body, both verbatim.
//
// A Kubernetes Status is the core's answer to success and to failure alike, and the caller relays it to
// its own client unchanged rather than re-deriving one: a second mapping of the same condition would
// drift from the core's. err is therefore reserved for a transport failure, where no response was
// obtained at all and there is nothing to relay.
func (c *Client) UploadManifests(ctx context.Context, snapshotContentName string, manifests json.RawMessage) (int, []byte, error) {
	body, err := json.Marshal(manifestsUploadBody{Manifests: manifests})
	if err != nil {
		return 0, nil, fmt.Errorf("marshal manifests-upload body for SnapshotContent %q: %w", snapshotContentName, err)
	}

	result := c.restClient.Post().
		Resource("snapshotcontents").
		Name(snapshotContentName).
		SubResource("manifests-upload").
		SetHeader("Content-Type", runtime.ContentTypeJSON).
		Body(body).
		Do(ctx)

	var code int
	result.StatusCode(&code)
	raw, rawErr := result.Raw()
	if code == 0 {
		return 0, nil, fmt.Errorf("upload manifests for SnapshotContent %q: %w", snapshotContentName, rawErr)
	}
	return code, raw, nil
}
