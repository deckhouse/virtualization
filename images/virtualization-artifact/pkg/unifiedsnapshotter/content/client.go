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

// Package content is the single transport to the state-snapshotter core's manifests-download subresource.
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

func (c *Client) DownloadManifests(ctx context.Context, snapshotContentName string) ([]RawManifest, error) {
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
