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
	"errors"
	"io"
	"testing"

	"kubevirt.io/containerized-data-importer/pkg/image"
)

// Classification reads a 512-byte window; the remaining bytes exercise stream
// restoration. Keeping this target at 4 KiB also bounds fragmented reads.
const peekFuzzMaxInputSize = 4 << 10

// These signatures pin the formats for which the uploader must avoid applying
// another compression layer. They do not validate native image metadata.
var peekImageSignatures = []struct {
	offset int
	magic  string
}{
	{0, "QFI\xfb"},
	{0, "\x1f\x8b"},
	{0, "\x28\xb5\x2f\xfd"},
	{0, "\xfd7zXZ\x00"},
	{0, "KDMV"},
	{0x40, "\x7f\x10\xda\xbe"},
	{0, "conectix"},
	{0, "vhdxfile"},
	{0x101, "ustar"},
}

// FuzzPeekRawImage mutates image bytes and source read boundaries while running
// the production Go classifier. No external parser or temporary file is used.
func FuzzPeekRawImage(f *testing.F) {
	f.Add([]byte{}, uint16(1))
	f.Add([]byte{0}, uint16(1))
	f.Add([]byte("raw image"), uint16(3))
	f.Add(bytes.Repeat([]byte{0}, 4096), uint16(512))
	f.Add(bytes.Repeat([]byte{0xff}, 4096), uint16(17))
	for _, signature := range peekImageSignatures {
		short := make([]byte, signature.offset+len(signature.magic))
		copy(short[signature.offset:], signature.magic)
		f.Add(short, uint16(1))
		f.Add(short[:len(short)-1], uint16(17))
		padded := make([]byte, image.MaxExpectedHdrSize+1)
		copy(padded[signature.offset:], signature.magic)
		f.Add(padded, uint16(512))
	}

	f.Fuzz(func(t *testing.T, data []byte, chunkSize uint16) {
		if len(data) > peekFuzzMaxInputSize {
			t.Skip()
		}

		closeErr := errors.New("source close error")
		source := &peekFuzzSource{
			Reader:   bytes.NewReader(data),
			chunk:    1 + int(chunkSize)%4096,
			closeErr: closeErr,
		}
		isRaw, restored, err := peekRawImage(source)
		if err != nil {
			t.Fatalf("finite byte stream rejected: %v", err)
		}
		if restored == nil {
			t.Fatal("classification returned no restored stream")
		}
		if consumed := len(data) - source.Len(); consumed > image.MaxExpectedHdrSize {
			t.Fatalf("classification consumed %d bytes, header window is %d", consumed, image.MaxExpectedHdrSize)
		}
		if source.closes != 0 {
			t.Fatal("classification closed the source before returning it")
		}

		var header [512]byte
		copy(header[:], data)
		wantRaw := true
		for _, signature := range peekImageSignatures {
			if bytes.HasPrefix(header[signature.offset:], []byte(signature.magic)) {
				wantRaw = false
				break
			}
		}
		if isRaw != wantRaw {
			t.Fatalf("raw classification = %t, want %t", isRaw, wantRaw)
		}

		got, readErr := io.ReadAll(restored)
		closedErr := restored.Close()
		if readErr != nil {
			t.Fatalf("restored stream read failed: %v", readErr)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("restored stream differs from input: got %d bytes, want %d", len(got), len(data))
		}
		if !errors.Is(closedErr, closeErr) || source.closes != 1 {
			t.Fatalf("source close calls = %d, error = %v", source.closes, closedErr)
		}
	})
}

type peekFuzzSource struct {
	*bytes.Reader
	chunk    int
	closes   int
	closeErr error
}

func (r *peekFuzzSource) Read(p []byte) (int, error) {
	return r.Reader.Read(p[:min(len(p), r.chunk)])
}

func (r *peekFuzzSource) Close() error {
	r.closes++
	return r.closeErr
}
