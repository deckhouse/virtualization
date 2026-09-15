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

package restore

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/content"
)

// contentFetcher adapts the shared manifests-download transport to NodeManifestFetcher.
type contentFetcher struct {
	client *content.Client
}

var _ NodeManifestFetcher = &contentFetcher{}

// NewContentFetcher adapts an already-built content client, so the apiserver keeps one transport to the
// core rather than one per subresource.
func NewContentFetcher(client *content.Client) NodeManifestFetcher {
	return &contentFetcher{client: client}
}

func (f *contentFetcher) NodeBaseManifests(ctx context.Context, contentName string) ([]unstructured.Unstructured, error) {
	manifests, err := f.client.DownloadManifests(ctx, contentName)
	if err != nil {
		return nil, err
	}

	objs := make([]unstructured.Unstructured, 0, len(manifests))
	for _, m := range manifests {
		var obj unstructured.Unstructured
		if err := json.Unmarshal(m.Raw, &obj.Object); err != nil {
			return nil, fmt.Errorf("decode %s manifest from SnapshotContent %q: %w", m.Kind, contentName, err)
		}
		objs = append(objs, obj)
	}
	return objs, nil
}
