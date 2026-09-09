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

package importer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// A disk image is packed twice on its way to a volume: the registry layer and the disk file
// itself. The axes are exercised together: disk format x file compression x layer compression
// x write path. Whether a guest boots from the result needs a cluster.

const markerOffset = 512

var (
	diskFormats  = []string{"raw", "qcow2", "vmdk"}
	compressions = []string{"none", "gz", "xz", "zstd"}
	layerPacking = []string{"tar", "tar+zstd"}
	// A volume is either a block device (raw) or a file (qcow2); vmdk is a source format
	// only, so it never matches a target and always takes the scratch path.
	targetFormats = []string{"raw", "qcow2"}
)

func TestImportMatrixScratchPath(t *testing.T) {
	forEachCombination(t, func(t *testing.T, format string, disk, layer []byte) {
		t.Helper()
		destDir := t.TempDir()
		found, err := processLayer(context.Background(), nil, blobSource{blob: layer},
			types.BlobInfo{Digest: "sha256:test", Size: int64(len(layer))}, destDir, "disk", nil, true)
		if err != nil {
			t.Fatalf("processLayer: %v", err)
		}
		if !found {
			t.Fatal("disk file not found in the layer")
		}
		got := readOnlyFile(t, filepath.Join(destDir, "disk"))
		assertDisk(t, got, disk)
	})
}

func TestImportMatrixDirectPath(t *testing.T) {
	forEachCombination(t, func(t *testing.T, format string, disk, layer []byte) {
		t.Helper()
		for _, target := range targetFormats {
			t.Run("target-"+target, func(t *testing.T) {
				restore := stubTargetFormat(target)
				defer restore()

				destFile := filepath.Join(t.TempDir(), "target")
				found, err := processLayerToFile(context.Background(), blobSource{blob: layer},
					types.BlobInfo{Digest: "sha256:test", Size: int64(len(layer))}, destFile, "disk", nil)

				// The direct path is only taken when the formats already match; a
				// mismatch must be refused rather than written out.
				if target != format {
					if err == nil {
						t.Fatalf("format %s onto %s target: expected refusal, got none", format, target)
					}
					if !strings.Contains(err.Error(), "does not match target format") {
						t.Fatalf("unexpected error: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("processLayerToFile: %v", err)
				}
				if !found {
					t.Fatal("disk file not found in the layer")
				}
				got, err := os.ReadFile(destFile)
				if err != nil {
					t.Fatal(err)
				}
				assertDisk(t, got, disk)
			})
		}
	})
}

func forEachCombination(t *testing.T, check func(t *testing.T, format string, disk, layer []byte)) {
	t.Helper()
	for _, format := range diskFormats {
		for _, compression := range compressions {
			for _, packing := range layerPacking {
				name := fmt.Sprintf("%s/%s/%s", format, compression, packing)
				t.Run(name, func(t *testing.T) {
					disk := diskImage(format)
					file := compressBytes(t, compression, disk)
					layer := packLayer(t, packing, "disk/image."+format+extFor(compression), file)
					check(t, format, disk, layer)
				})
			}
		}
	}
}

// diskImage builds a disk file recognisable by its header; the readers key off the magic and
// the size field. Mostly zeroes on purpose: that makes a compressor read its source in small
// steps, which is what a shared header buffer used to corrupt.
func diskImage(format string) []byte {
	b := make([]byte, 4<<20)
	copy(b[markerOffset:], []byte("PTMARKER-"+format))
	switch format {
	case "qcow2":
		copy(b, []byte{'Q', 'F', 'I', 0xfb})
		binary.BigEndian.PutUint32(b[4:], 3)
		binary.BigEndian.PutUint64(b[24:], uint64(len(b)))
	case "vmdk":
		copy(b, []byte("KDMV"))
	}
	return b
}

func extFor(compression string) string {
	switch compression {
	case "gz":
		return ".gz"
	case "xz":
		return ".xz"
	case "zstd":
		return ".zst"
	}
	return ""
}

func compressBytes(t *testing.T, kind string, b []byte) []byte {
	t.Helper()
	if kind == "none" {
		return b
	}
	var out bytes.Buffer
	var w io.WriteCloser
	var err error
	switch kind {
	case "gz":
		w = gzip.NewWriter(&out)
	case "xz":
		w, err = xz.NewWriter(&out)
	case "zstd":
		w, err = zstd.NewWriter(&out, zstd.WithEncoderLevel(zstd.SpeedFastest))
	default:
		t.Fatalf("unknown compression %q", kind)
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func packLayer(t *testing.T, packing, name string, file []byte) []byte {
	t.Helper()
	var tarred bytes.Buffer
	tw := tar.NewWriter(&tarred)
	if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(file)), Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(file); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if packing == "tar" {
		return tarred.Bytes()
	}
	return compressBytes(t, "zstd", tarred.Bytes())
}

func assertDisk(t *testing.T, got, want []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	switch {
	case len(got) > 2 && got[0] == 0x1f && got[1] == 0x8b:
		t.Fatalf("target holds a gzip archive, not the disk image (%d bytes)", len(got))
	case len(got) > 6 && bytes.Equal(got[:6], []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}):
		t.Fatalf("target holds an xz archive, not the disk image (%d bytes)", len(got))
	case len(got) > 4 && bytes.Equal(got[:4], []byte{0x28, 0xb5, 0x2f, 0xfd}):
		t.Fatalf("target holds a zstd archive, not the disk image (%d bytes)", len(got))
	}
	t.Fatalf("disk image mismatch: got %d bytes, want %d", len(got), len(want))
}

func readOnlyFile(t *testing.T, dir string) []byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one file in %s, got %d", dir, len(entries))
	}
	b, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// blobSource serves one layer blob and nothing else: the layer readers only call GetBlob.
type blobSource struct {
	types.ImageSource
	blob []byte
}

func (s blobSource) GetBlob(_ context.Context, _ types.BlobInfo, _ types.BlobInfoCache) (io.ReadCloser, int64, error) {
	return io.NopCloser(bytes.NewReader(s.blob)), int64(len(s.blob)), nil
}

func stubTargetFormat(format string) func() {
	prev := targetFormatOf
	targetFormatOf = func(string) (string, error) { return format, nil }
	return func() { targetFormatOf = prev }
}
