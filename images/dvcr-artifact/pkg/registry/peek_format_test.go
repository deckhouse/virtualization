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
)

func TestPeekRawImage(t *testing.T) {
	qcow2 := append([]byte{'Q', 'F', 'I', 0xfb}, bytes.Repeat([]byte{0}, 4096)...)
	gz := append([]byte{0x1f, 0x8b}, bytes.Repeat([]byte{0}, 4096)...)

	tests := map[string]struct {
		payload []byte
		wantRaw bool
	}{
		"raw image":           {payload: bytes.Repeat([]byte{0xAB}, 4096), wantRaw: true},
		"all zeroes":          {payload: bytes.Repeat([]byte{0x00}, 4096), wantRaw: true},
		"qcow2":               {payload: qcow2, wantRaw: false},
		"gzip":                {payload: gz, wantRaw: false},
		"shorter than header": {payload: []byte("tiny"), wantRaw: true},
		"empty":               {payload: nil, wantRaw: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			isRaw, rc, err := peekRawImage(io.NopCloser(bytes.NewReader(tt.payload)))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if isRaw != tt.wantRaw {
				t.Fatalf("isRaw = %t, want %t", isRaw, tt.wantRaw)
			}

			// Whatever was consumed to classify must still be readable.
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("reading restored stream: %v", err)
			}
			if !bytes.Equal(got, tt.payload) {
				t.Fatalf("restored stream lost data: got %d bytes, want %d", len(got), len(tt.payload))
			}
		})
	}
}

func TestPeekRawImageClosesSource(t *testing.T) {
	src := &closeTracker{Reader: bytes.NewReader(bytes.Repeat([]byte{1}, 1024))}
	_, rc, err := peekRawImage(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if !src.closed {
		t.Fatal("closing the restored reader must close the original source")
	}
}

type closeTracker struct {
	io.Reader
	closed bool
}

func (c *closeTracker) Close() error {
	c.closed = true
	return nil
}
