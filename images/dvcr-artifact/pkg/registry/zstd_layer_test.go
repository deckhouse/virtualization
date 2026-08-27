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

	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
)

func TestZstdLayerRoundTrip(t *testing.T) {
	// A payload shaped like a disk image: mostly zeroes with some data.
	payload := append(bytes.Repeat([]byte{0x42}, 1<<20), bytes.Repeat([]byte{0}, 8<<20)...)

	l := newZstdLayer(io.NopCloser(bytes.NewReader(payload)))

	mt, err := l.MediaType()
	if err != nil {
		t.Fatal(err)
	}
	if mt != types.OCILayerZStd {
		t.Fatalf("media type = %q, want %q", mt, types.OCILayerZStd)
	}

	// Digests are unknown until the stream has been consumed: that is what makes
	// the remote pusher take its streaming path.
	if _, err := l.Digest(); !errors.Is(err, stream.ErrNotComputed) {
		t.Fatalf("Digest before consuming = %v, want ErrNotComputed", err)
	}

	rc, err := l.Compressed()
	if err != nil {
		t.Fatal(err)
	}
	blob, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}

	// The blob must decompress back to exactly the input.
	dec, err := zstd.NewReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()
	got, err := io.ReadAll(dec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("round trip mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// A zero-heavy payload must actually shrink; otherwise the whole point is lost.
	if len(blob) >= len(payload)/2 {
		t.Fatalf("blob did not shrink: %d bytes from %d", len(blob), len(payload))
	}

	// Digest covers the compressed blob, DiffID the original stream.
	digest, err := l.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if want := sha256Hex(blob); digest.Hex != want {
		t.Fatalf("Digest = %s, want sha256 of the blob %s", digest.Hex, want)
	}

	diffID, err := l.DiffID()
	if err != nil {
		t.Fatal(err)
	}
	if want := sha256Hex(payload); diffID.Hex != want {
		t.Fatalf("DiffID = %s, want sha256 of the payload %s", diffID.Hex, want)
	}

	size, err := l.Size()
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(blob)) {
		t.Fatalf("Size = %d, want %d", size, len(blob))
	}
}

func TestZstdLayerConsumedOnce(t *testing.T) {
	l := newZstdLayer(io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{7}, 4096))))

	rc, err := l.Compressed()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatal(err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := l.Compressed(); !errors.Is(err, stream.ErrConsumed) {
		t.Fatalf("second Compressed() = %v, want ErrConsumed", err)
	}
}

func TestZstdLayerPropagatesSourceError(t *testing.T) {
	errBoom := errors.New("source boom")
	src := io.MultiReader(bytes.NewReader(bytes.Repeat([]byte{1}, 4096)), errReader{errBoom})

	l := newZstdLayer(io.NopCloser(src))
	rc, err := l.Compressed()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(rc); !errors.Is(err, errBoom) {
		t.Fatalf("read error = %v, want %v", err, errBoom)
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }
