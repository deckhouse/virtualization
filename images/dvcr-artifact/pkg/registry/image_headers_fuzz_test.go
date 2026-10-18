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
	"io"
	"testing"

	"github.com/golang/snappy"
	"kubevirt.io/containerized-data-importer/pkg/image"
)

// Pin the signatures of the CDI revision used by release-1.10. The target
// calls CDI's real Header.Match method; it does not introduce peekRawImage.
var releaseHeaderSignatures = []struct {
	format string
	offset int
	magic  string
}{
	{"qcow2", 0, "QFI\xfb"},
	{"gz", 0, "\x1f\x8b"},
	{"zst", 0, "\x28\xb5\x2f\xfd"},
	{"xz", 0, "\xfd7zXZ\x00"},
	{"vmdk", 0, "KDMV"},
	{"vdi", 0x40, "\x7f\x10\xda\xbe"},
	{"vhd", 0, "conectix"},
	{"vhdx", 0, "vhdxfile"},
	{"tar", 0x101, "ustar"},
}

// FuzzImageHeaders replaces the native image inspection target with the
// header matching code already used by release getImageInfo and CDI's
// FormatReaders. No image metadata inspection, decompression, subprocess,
// temporary file, or production implementation of raw-image detection runs.
func FuzzImageHeaders(f *testing.F) {
	for _, seed := range releaseHeaderSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > image.MaxExpectedHdrSize {
			t.Skip()
		}

		// Header.Match requires a full window. Short fuzz values represent
		// zero-filled header windows, not successful parsing of short files.
		// Passing arbitrary short slices directly would violate its contract.
		var header [image.MaxExpectedHdrSize]byte
		copy(header[:], data)
		checkReleaseHeaderMatches(t, header)
	})
}

// FuzzSnappyImageHeaders replaces FuzzImageInfoSnappy when native image
// tools are excluded. Snappy framing and header matching remain under test;
// only the first header window is read, without inspecting image metadata.
func FuzzSnappyImageHeaders(f *testing.F) {
	for _, seed := range releaseHeaderSeeds() {
		var framed bytes.Buffer
		writer := snappy.NewBufferedWriter(&framed)
		if _, err := writer.Write(seed); err != nil {
			f.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			f.Fatal(err)
		}
		f.Add(append([]byte(nil), framed.Bytes()...))
	}
	f.Add([]byte{})
	f.Add([]byte("\xff\x06\x00\x00sNaPpY"))
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\xff\xff\xff"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}

		var header [image.MaxExpectedHdrSize]byte
		reader := snappy.NewReader(bytes.NewReader(data))
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			// Malformed frames and streams shorter than one full header are
			// rejected, as they are by the release FormatReaders caller.
			return
		}
		checkReleaseHeaderMatches(t, header)
	})
}

func checkReleaseHeaderMatches(t *testing.T, header [image.MaxExpectedHdrSize]byte) {
	t.Helper()

	headers := image.CopyKnownHdrs()
	if len(headers) != len(releaseHeaderSignatures) {
		t.Fatalf("CDI header definitions changed: got %d, want %d", len(headers), len(releaseHeaderSignatures))
	}
	original := header
	for _, signature := range releaseHeaderSignatures {
		h, ok := headers[signature.format]
		if !ok || h.Format != signature.format {
			t.Fatalf("missing or inconsistent definition of %s", signature.format)
		}
		end := signature.offset + len(signature.magic)
		want := string(original[signature.offset:end]) == signature.magic
		if got := h.Match(header[:]); got != want {
			t.Fatalf("%s header match = %t, want %t", signature.format, got, want)
		}
		if header != original {
			t.Fatalf("%s matching modified the input header", signature.format)
		}
	}
}

func releaseHeaderSeeds() [][]byte {
	seeds := [][]byte{
		{},
		{0},
		[]byte("raw image"),
		make([]byte, image.MaxExpectedHdrSize),
		bytes.Repeat([]byte{0xff}, image.MaxExpectedHdrSize),
	}
	for _, signature := range releaseHeaderSignatures {
		full := make([]byte, image.MaxExpectedHdrSize)
		copy(full[signature.offset:], signature.magic)
		end := signature.offset + len(signature.magic)
		seeds = append(seeds, full, append([]byte(nil), full[:end]...), append([]byte(nil), full[:end-1]...))
	}
	// Multiple signatures can coexist. Test every matcher without imposing
	// a priority on the release map's deliberately unspecified iteration order.
	combined := make([]byte, image.MaxExpectedHdrSize)
	copy(combined, "QFI\xfb")
	copy(combined[0x40:], "\x7f\x10\xda\xbe")
	copy(combined[0x101:], "ustar")
	return append(seeds, combined)
}
