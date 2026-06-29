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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func Test_UncompressedLayer_MediaType(t *testing.T) {
	l := newUncompressedLayer(io.NopCloser(bytes.NewReader(nil)))
	mt, err := l.MediaType()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mt != types.DockerUncompressedLayer {
		t.Fatalf("media type: got %q, want %q", mt, types.DockerUncompressedLayer)
	}
}

func Test_UncompressedLayer_NotComputedBeforeConsumed(t *testing.T) {
	l := newUncompressedLayer(io.NopCloser(bytes.NewReader([]byte("data"))))

	if _, err := l.Digest(); !errors.Is(err, stream.ErrNotComputed) {
		t.Fatalf("Digest before consume: got %v, want stream.ErrNotComputed", err)
	}
	if _, err := l.DiffID(); !errors.Is(err, stream.ErrNotComputed) {
		t.Fatalf("DiffID before consume: got %v, want stream.ErrNotComputed", err)
	}
	if _, err := l.Size(); !errors.Is(err, stream.ErrNotComputed) {
		t.Fatalf("Size before consume: got %v, want stream.ErrNotComputed", err)
	}
}

func Test_UncompressedLayer_StreamsRawBytesAndComputesDigest(t *testing.T) {
	payload := bytes.Repeat([]byte("virtualization-disk-image"), 4096)

	l := newUncompressedLayer(io.NopCloser(bytes.NewReader(payload)))

	rc, err := l.Compressed()
	if err != nil {
		t.Fatalf("Compressed: %v", err)
	}

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The uploaded bytes must equal the input: no compression is applied.
	if !bytes.Equal(got, payload) {
		t.Fatalf("streamed bytes differ from input: got %d bytes, want %d bytes", len(got), len(payload))
	}

	wantDigest := "sha256:" + hex.EncodeToString(sum256(payload))

	digest, err := l.Digest()
	if err != nil {
		t.Fatalf("Digest after consume: %v", err)
	}
	if digest.String() != wantDigest {
		t.Fatalf("digest: got %q, want %q", digest.String(), wantDigest)
	}

	// For an uncompressed layer DiffID equals Digest.
	diffID, err := l.DiffID()
	if err != nil {
		t.Fatalf("DiffID after consume: %v", err)
	}
	if diffID != digest {
		t.Fatalf("diffID %q != digest %q", diffID, digest)
	}

	size, err := l.Size()
	if err != nil {
		t.Fatalf("Size after consume: %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("size: got %d, want %d", size, len(payload))
	}
}

func Test_UncompressedLayer_SecondReadFailsAfterConsumed(t *testing.T) {
	l := newUncompressedLayer(io.NopCloser(bytes.NewReader([]byte("payload"))))

	rc, err := l.Compressed()
	if err != nil {
		t.Fatalf("first Compressed: %v", err)
	}
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := l.Compressed(); !errors.Is(err, stream.ErrConsumed) {
		t.Fatalf("second Compressed: got %v, want stream.ErrConsumed", err)
	}
}

// satisfy the v1.Layer interface at compile time in the test too.
var _ v1.Layer = (*uncompressedLayer)(nil)

func sum256(b []byte) []byte {
	h := sha256.New()
	h.Write(b)
	return h.Sum(nil)
}
