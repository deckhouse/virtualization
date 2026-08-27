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

package registry

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// A zstd layer must land in an OCI manifest: readers that follow the docker
// v2s2 schema reject the zstd media type outright, before reading the blob, so
// mixing the two makes the image unusable for the importer.
func TestUploadedImageManifestMediaType(t *testing.T) {
	tests := []struct {
		name         string
		compress     bool
		wantManifest types.MediaType
		wantLayer    types.MediaType
	}{
		{
			name:         "compressed layer goes up as OCI",
			compress:     true,
			wantManifest: types.OCIManifestSchema1,
			wantLayer:    types.OCILayerZStd,
		},
		{
			name:         "uncompressed layer keeps the docker manifest",
			compress:     false,
			wantManifest: types.DockerManifestSchema2,
			wantLayer:    types.DockerUncompressedLayer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(registry.New())
			defer srv.Close()

			payload := append(bytes.Repeat([]byte{0x42}, 1<<10), bytes.Repeat([]byte{0}, 4<<10)...)
			imageName := strings.TrimPrefix(srv.URL, "http://") + "/disk:test"

			p := DataProcessor{destImageName: imageName, destInsecure: true}
			informer := NewImageInformer()
			informer.Set(uint64(len(payload)), "raw")

			err := p.uploadLayersAndImage(
				context.Background(),
				io.NopCloser(bytes.NewReader(payload)),
				len(payload),
				informer,
				tt.compress,
			)
			if err != nil {
				t.Fatalf("upload: %v", err)
			}

			ref, err := name.ParseReference(imageName, name.Insecure)
			if err != nil {
				t.Fatal(err)
			}
			img, err := remote.Image(ref)
			if err != nil {
				t.Fatalf("read back the image: %v", err)
			}

			mt, err := img.MediaType()
			if err != nil {
				t.Fatal(err)
			}
			if mt != tt.wantManifest {
				t.Errorf("manifest media type: got %q, want %q", mt, tt.wantManifest)
			}

			manifest, err := img.Manifest()
			if err != nil {
				t.Fatal(err)
			}
			if len(manifest.Layers) != 1 {
				t.Fatalf("got %d layers, want 1", len(manifest.Layers))
			}
			if got := manifest.Layers[0].MediaType; got != tt.wantLayer {
				t.Errorf("layer media type: got %q, want %q", got, tt.wantLayer)
			}
		})
	}
}
