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
	"context"
	"io"
	"testing"
)

// patternReader yields a deterministic non-zero byte stream of a given size.
// The pattern matches no known image header, so qemu-img classifies it as raw.
type patternReader struct {
	size int64
	off  int64
}

func (r *patternReader) Read(p []byte) (int, error) {
	if r.off >= r.size {
		return 0, io.EOF
	}
	n := len(p)
	if remaining := r.size - r.off; int64(n) > remaining {
		n = int(remaining)
	}
	for i := 0; i < n; i++ {
		p[i] = byte((r.off + int64(i)) % 251)
	}
	r.off += int64(n)
	return n, nil
}

// TestGetImageInfoRawVirtualSize pins the raw-source size accounting: every
// source byte must be counted exactly once. A regression that double-counts
// the probed header (the first syntheticHeadSize bytes are prepended to the
// stream again by getImageInfo) inflates VirtualSize by exactly 10 MiB, which
// in turn inflates VirtualImage unpackedSize for block-device sources and
// breaks size validation of disks cloned from such images.
func TestGetImageInfoRawVirtualSize(t *testing.T) {
	cases := []struct {
		name string
		size int64
	}{
		{name: "smaller than the probed header", size: 3*1024*1024 + 17},
		{name: "larger than the probed header", size: syntheticHeadSize + 5*1024*1024 + 123},
		{name: "larger than the sampled temp file", size: imageInfoSize + 6*1024*1024 + 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := io.NopCloser(&patternReader{size: tc.size})

			info, err := getImageInfo(context.Background(), src)
			if err != nil {
				t.Fatalf("getImageInfo failed: %v", err)
			}
			if info.Format != "raw" {
				t.Fatalf("unexpected format: got %q, want %q", info.Format, "raw")
			}
			if info.VirtualSize != uint64(tc.size) {
				t.Fatalf("VirtualSize miscounted: got %d, want %d (diff %d)",
					info.VirtualSize, tc.size, int64(info.VirtualSize)-tc.size)
			}
		})
	}
}
