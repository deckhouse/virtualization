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
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyBufferedWriteSizes(t *testing.T) {
	// All writes except the last must be exactly copyBufferSize: that is the
	// whole point of the change (no short unaligned writes to the device).
	data := bytes.Repeat([]byte{0x01}, copyBufferSize*2+777)

	sources := map[string]func() io.Reader{
		// Whole stream in one Read: catches the bytes.Reader.WriteTo bypass
		// that broke the bufio.Writer variant.
		"single read": func() io.Reader { return bytes.NewReader(data) },
		// Registry transport pattern: the buffer must be filled from many
		// short reads instead of forwarding them as they arrive.
		"short reads": func() io.Reader {
			return &shortReadReader{r: bytes.NewReader(data), max: 32 * 1024}
		},
	}

	for name, newSource := range sources {
		t.Run(name, func(t *testing.T) {
			var sizes []int
			w := shortWriter{onWrite: func(n int) { sizes = append(sizes, n) }}
			if err := copyBuffered(w, newSource()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(sizes) != 3 {
				t.Fatalf("got %d writes, want 3: %v", len(sizes), sizes)
			}
			for i, s := range sizes[:len(sizes)-1] {
				if s != copyBufferSize {
					t.Fatalf("write %d size %d, want %d", i, s, copyBufferSize)
				}
			}
			if last := sizes[len(sizes)-1]; last != 777 {
				t.Fatalf("last write size %d, want 777", last)
			}
		})
	}
}

func TestCopyBufferedReadError(t *testing.T) {
	// Reader fails mid-stream: error must propagate.
	errBoom := errors.New("boom")
	data := bytes.Repeat([]byte{0x02}, copyBufferSize+512)
	r := io.MultiReader(bytes.NewReader(data), errReader{errBoom})
	if err := copyBuffered(io.Discard, r); !errors.Is(err, errBoom) {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestCopyBufferedWriteError(t *testing.T) {
	errBoom := errors.New("write boom")
	data := bytes.Repeat([]byte{0x03}, copyBufferSize*2)
	if err := copyBuffered(failWriter{errBoom}, bytes.NewReader(data)); !errors.Is(err, errBoom) {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestCopyBufferedTruncatedSource(t *testing.T) {
	// A transfer cut off mid-stream must fail. gzip, tar and net/http all
	// report a truncated stream as io.ErrUnexpectedEOF, which is also what
	// io.ReadFull returns for a legitimate short tail: mixing the two up turns
	// a broken import into a Ready disk with missing data.
	payload := bytes.Repeat([]byte{0xAB}, copyBufferSize*2+55)

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	zr, err := gzip.NewReader(bytes.NewReader(compressed.Bytes()[:compressed.Len()-20]))
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := copyBuffered(&got, zr); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("want ErrUnexpectedEOF, got %v (wrote %d of %d bytes)", err, got.Len(), len(payload))
	}
}

func TestStreamDataToFileTailIntact(t *testing.T) {
	// End-to-end through streamDataToFile: the file on disk must match the
	// source byte-for-byte, including the sub-buffer tail.
	data := bytes.Repeat([]byte{0x04}, copyBufferSize+12345)
	dir := t.TempDir()
	f := filepath.Join(dir, "dest.img")
	if err := streamDataToFile(bytes.NewReader(data), f); err != nil {
		t.Fatalf("streamDataToFile: %v", err)
	}
	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("file content mismatch: len %d, want %d", len(got), len(data))
	}
}

type shortWriter struct {
	onWrite func(n int)
}

func (w shortWriter) Write(p []byte) (int, error) {
	if w.onWrite != nil {
		w.onWrite(len(p))
	}
	return len(p), nil
}

type failWriter struct {
	err error
}

func (w failWriter) Write(p []byte) (int, error) { return 0, w.err }

type errReader struct {
	err error
}

func (r errReader) Read(p []byte) (int, error) { return 0, r.err }

// shortReadReader caps every Read at max bytes, like a network stream.
type shortReadReader struct {
	r   io.Reader
	max int
}

func (r *shortReadReader) Read(p []byte) (int, error) {
	if len(p) > r.max {
		p = p[:r.max]
	}
	return r.r.Read(p)
}
