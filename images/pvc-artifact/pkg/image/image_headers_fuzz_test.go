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

package image_test

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
	// Keep each certification seed as a separate, explicit f.Add call.
	// Empty header.
	f.Add([]byte{})
	// Single zero byte.
	f.Add([]byte("\x00"))
	// Unrecognized raw image bytes.
	f.Add([]byte("raw image"))
	// Zero-filled 512-byte header.
	f.Add([]byte{511: 0x00})
	// 0xff-filled 512-byte header.
	f.Add([]byte("\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff" +
		"\xff\xff\xff\xff\xff\xff\xff\xff"))
	// qcow2: full 512-byte window.
	f.Add([]byte{0: 0x51, 1: 0x46, 2: 0x49, 3: 0xfb, 511: 0x00})
	// qcow2: complete signature.
	f.Add([]byte("QFI\xfb"))
	// qcow2: signature truncated by one byte.
	f.Add([]byte("QFI"))
	// gzip: full 512-byte window.
	f.Add([]byte{0: 0x1f, 1: 0x8b, 511: 0x00})
	// gzip: complete signature.
	f.Add([]byte("\x1f\x8b"))
	// gzip: signature truncated by one byte.
	f.Add([]byte("\x1f"))
	// zstd: full 512-byte window.
	f.Add([]byte{0: 0x28, 1: 0xb5, 2: 0x2f, 3: 0xfd, 511: 0x00})
	// zstd: complete signature.
	f.Add([]byte("(\xb5/\xfd"))
	// zstd: signature truncated by one byte.
	f.Add([]byte("(\xb5/"))
	// xz: full 512-byte window.
	f.Add([]byte{0: 0xfd, 1: 0x37, 2: 0x7a, 3: 0x58, 4: 0x5a, 511: 0x00})
	// xz: complete signature.
	f.Add([]byte("\xfd7zXZ\x00"))
	// xz: signature truncated by one byte.
	f.Add([]byte("\xfd7zXZ"))
	// vmdk: full 512-byte window.
	f.Add([]byte{0: 0x4b, 1: 0x44, 2: 0x4d, 3: 0x56, 511: 0x00})
	// vmdk: complete signature.
	f.Add([]byte("KDMV"))
	// vmdk: signature truncated by one byte.
	f.Add([]byte("KDM"))
	// vdi: full 512-byte window.
	f.Add([]byte{64: 0x7f, 65: 0x10, 66: 0xda, 67: 0xbe, 511: 0x00})
	// vdi: complete signature.
	f.Add([]byte{64: 0x7f, 65: 0x10, 66: 0xda, 67: 0xbe})
	// vdi: signature truncated by one byte.
	f.Add([]byte{64: 0x7f, 65: 0x10, 66: 0xda})
	// vhd: full 512-byte window.
	f.Add([]byte{0: 0x63, 1: 0x6f, 2: 0x6e, 3: 0x65, 4: 0x63, 5: 0x74, 6: 0x69, 7: 0x78, 511: 0x00})
	// vhd: complete signature.
	f.Add([]byte("conectix"))
	// vhd: signature truncated by one byte.
	f.Add([]byte("conecti"))
	// vhdx: full 512-byte window.
	f.Add([]byte{0: 0x76, 1: 0x68, 2: 0x64, 3: 0x78, 4: 0x66, 5: 0x69, 6: 0x6c, 7: 0x65, 511: 0x00})
	// vhdx: complete signature.
	f.Add([]byte("vhdxfile"))
	// vhdx: signature truncated by one byte.
	f.Add([]byte("vhdxfil"))
	// tar: full 512-byte window.
	f.Add([]byte{257: 0x75, 258: 0x73, 259: 0x74, 260: 0x61, 261: 0x72, 511: 0x00})
	// tar: complete signature.
	f.Add([]byte{257: 0x75, 258: 0x73, 259: 0x74, 260: 0x61, 261: 0x72})
	// tar: signature truncated by one byte.
	f.Add([]byte{257: 0x75, 258: 0x73, 259: 0x74, 260: 0x61})
	// Coexisting qcow2, vdi and tar signatures.
	f.Add([]byte{0: 0x51, 1: 0x46, 2: 0x49, 3: 0xfb, 64: 0x7f, 65: 0x10, 66: 0xda, 67: 0xbe, 257: 0x75, 258: 0x73, 259: 0x74, 260: 0x61, 261: 0x72, 511: 0x00})

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
	// Keep each certification seed as a separate, explicit f.Add call.
	// Snappy frame for empty header.
	f.Add([]byte{})
	// Snappy frame for single zero byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x05\x00\x00\xd2\x8f%I\x00"))
	// Snappy frame for unrecognized raw image bytes.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x0d\x00\x00K\x96\x1c\x1fraw im" +
		"age"))
	// Snappy frame for zero-filled 512-byte header.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00 \x00\x00\xd1L\x03~\x80\x04\x00\x00\xfe\x01" +
		"\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfa\x01\x00"))
	// Snappy frame for 0xff-filled 512-byte header.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00 \x00\x00\x8b\xa2\xb1\xc7\x80\x04\x00\xff\xfe\x01" +
		"\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfa\x01\x00"))
	// Snappy frame for qcow2: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00$\x00\x00\xb5\xb3\x1e%\x80\x04\x10QFI" +
		"\xfb\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xea" +
		"\x01\x00"))
	// Snappy frame for qcow2: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x08\x00\x00\xbc\x9f`\xe4QFI\xfb"))
	// Snappy frame for qcow2: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x07\x00\x00\xbf\xa6\xa6\xdcQFI"))
	// Snappy frame for gzip: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00\"\x00\x00\xe0\x16s|\x80\x04\x08\x1f\x8b\x00" +
		"\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xf2\x01\x00"))
	// Snappy frame for gzip: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x06\x00\x00\x1cP]\xdc\x1f\x8b"))
	// Snappy frame for gzip: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x05\x00\x009#\xb7\xf2\x1f"))
	// Snappy frame for zstd: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00$\x00\x00=\x03.\xfc\x80\x04\x10(\xb5/" +
		"\xfd\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xea" +
		"\x01\x00"))
	// Snappy frame for zstd: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x08\x00\x00\xb0$\x18\x05(\xb5/\xfd"))
	// Snappy frame for zstd: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x07\x00\x00\xcd-\x83-(\xb5/"))
	// Snappy frame for xz: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00%\x00\x00/\xd6j\x8c\x80\x04\x14\xfd7z" +
		"XZ\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00" +
		"\xe6\x01\x00"))
	// Snappy frame for xz: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x0a\x00\x009;\x0f\xce\xfd7zXZ\x00"))
	// Snappy frame for xz: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x09\x00\x00c\xd4Z\x10\xfd7zXZ"))
	// Snappy frame for vmdk: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00$\x00\x00\x02\xeb\xd9\x1a\x80\x04\x10KDM" +
		"V\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xea" +
		"\x01\x00"))
	// Snappy frame for vmdk: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x08\x00\x00>\xb8\xa3\xd8KDMV"))
	// Snappy frame for vmdk: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x07\x00\x008`\xc7\xf9KDM"))
	// Snappy frame for vdi: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00%\x00\x00\x04,\xb7\x10\x80\x04\x00\x00\xfa\x01" +
		"\x00\x0c\x7f\x10\xda\xbe\xfaC\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00" +
		"\xf2\x01\x00"))
	// Snappy frame for vdi: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00\x0f\x00\x00\x1b\x859vD\x00\x00\xfa\x01\x00" +
		"\x0c\x7f\x10\xda\xbe"))
	// Snappy frame for vdi: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00\x0e\x00\x00\x0eTe\xc8C\x00\x00\xfa\x01\x00" +
		"\x08\x7f\x10\xda"))
	// Snappy frame for vhd: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00(\x00\x00\xc8\xaeU\xc9\x80\x04 con" +
		"ectix\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00" +
		"\xfe\x01\x00\xda\x01\x00"))
	// Snappy frame for vhd: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x0c\x00\x00\xd3\xee~\xbbconect" +
		"ix"))
	// Snappy frame for vhd: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x0b\x00\x00\x92H\x18\xc6conect" +
		"i"))
	// Snappy frame for vhdx: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00(\x00\x00L\xd1\xb3\xa7\x80\x04 vhd" +
		"xfile\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00" +
		"\xfe\x01\x00\xda\x01\x00"))
	// Snappy frame for vhdx: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x0c\x00\x00\xda\xe7|Nvhdxfi" +
		"le"))
	// Snappy frame for vhdx: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x01\x0b\x00\x00K\xf4Y\xb7vhdxfi" +
		"l"))
	// Snappy frame for tar: full 512-byte window.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00&\x00\x00\x1b\x08\x860\x80\x04\x00\x00\xfe\x01" +
		"\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\x10ustar\xfe\x05\x01\xfe\x05\x01\xfe\x05" +
		"\x01\xe6\x05\x01"))
	// Snappy frame for tar: complete signature.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00\x1a\x00\x00\xf3\xf7.\xeb\x86\x02\x00\x00\xfe\x01" +
		"\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\x10ustar"))
	// Snappy frame for tar: signature truncated by one byte.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x00\x19\x00\x00\xb8y\xb2\x94\x85\x02\x00\x00\xfe\x01" +
		"\x00\xfe\x01\x00\xfe\x01\x00\xfe\x01\x00\x0custa"))
	// Snappy frame for coexisting qcow2, vdi and tar signatures.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY\x003\x00\x00\x8a\x7f/\x05\x80\x04\x10QFI" +
		"\xfb\x00\xea\x01\x00\x0c\x7f\x10\xda\xbe\xea?\x00\xfe\x01\x00\xee\x01\x00\x09\x01\x10us" +
		"tar\xfe\x87\x00\xee\x87\x00\x09\x87\xfe\x01\x00\xde\x01\x00"))
	// empty encoded stream.
	f.Add([]byte{})
	// Snappy framing header without a data chunk.
	f.Add([]byte("\xff\x06\x00\x00sNaPpY"))
	// Snappy framing header with a truncated chunk header.
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
