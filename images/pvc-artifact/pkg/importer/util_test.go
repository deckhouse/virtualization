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

	"golang.org/x/sys/unix"
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

func TestCopyBufferedZeroBlocks(t *testing.T) {
	// Zero blocks must reach the destination as zeroing requests, everything
	// else as writes, and the offsets must account for both.
	data := bytes.Repeat([]byte{0x05}, copyBufferSize)                 // written
	data = append(data, make([]byte, copyBufferSize*2)...)             // zeroed
	data = append(data, bytes.Repeat([]byte{0x06}, copyBufferSize)...) // written
	data = append(data, make([]byte, 4096)...)                         // zeroed tail

	w := &recordingZeroWriter{}
	if err := copyBuffered(w, bytes.NewReader(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantZeroed := []zeroCall{
		{copyBufferSize, copyBufferSize},
		{copyBufferSize * 2, copyBufferSize},
		{copyBufferSize * 4, 4096},
	}
	if len(w.zeroed) != len(wantZeroed) {
		t.Fatalf("got %d zeroing calls, want %d: %v", len(w.zeroed), len(wantZeroed), w.zeroed)
	}
	for i, want := range wantZeroed {
		if w.zeroed[i] != want {
			t.Fatalf("zeroing call %d = %v, want %v", i, w.zeroed[i], want)
		}
	}
	if w.written != copyBufferSize*2 {
		t.Fatalf("wrote %d bytes, want %d", w.written, copyBufferSize*2)
	}
}

func TestCopyBufferedZeroRangeFallback(t *testing.T) {
	// A backend that refuses to zero ranges must not break the copy: the zeroes
	// get written instead, and the refusal is not retried for every block.
	data := make([]byte, copyBufferSize*3)

	w := &recordingZeroWriter{zeroErr: errors.New("operation not supported")}
	if err := copyBuffered(w, bytes.NewReader(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.zeroed) != 1 {
		t.Fatalf("zeroing attempted %d times, want 1 (then fall back for good)", len(w.zeroed))
	}
	if w.written != copyBufferSize*3 {
		t.Fatalf("wrote %d bytes, want %d", w.written, copyBufferSize*3)
	}
}

func TestStreamDataToFileZeroesOnDisk(t *testing.T) {
	// End-to-end through the real fallocate path: a file with a zero gap in the
	// middle must read back byte-identical to the source.
	data := bytes.Repeat([]byte{0x07}, copyBufferSize)
	data = append(data, make([]byte, copyBufferSize)...)
	data = append(data, bytes.Repeat([]byte{0x08}, 3000)...)

	f := filepath.Join(t.TempDir(), "zeroes.img")
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

func TestStreamDataToFileOverwritesLeftover(t *testing.T) {
	// An interrupted attempt leaves the target file on the volume; the importer pod
	// restarts on the same volume, so the retry must overwrite that leftover instead
	// of failing on it. The leftover is longer than the new image on purpose: none of
	// its tail may survive into the imported disk.
	data := bytes.Repeat([]byte{0x42}, copyBufferSize+1000)
	leftover := bytes.Repeat([]byte{0xAA}, len(data)*2)

	f := filepath.Join(t.TempDir(), "disk.img")
	if err := os.WriteFile(f, leftover, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := streamDataToFile(bytes.NewReader(data), f); err != nil {
		t.Fatalf("retry over a leftover file: %v", err)
	}
	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("file content mismatch: len %d, want %d", len(got), len(data))
	}
}

func TestCopyBufferedDropsPageCache(t *testing.T) {
	// Written data must be pushed out and evicted window by window, one window
	// behind the one being filled, so the pod cgroup never holds more than two.
	data := bytes.Repeat([]byte{0x09}, cacheWindowSize*3)

	w := &recordingCacheWriter{}
	if err := copyBuffered(w, bytes.NewReader(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []zeroCall{{0, cacheWindowSize}, {cacheWindowSize, cacheWindowSize}, {cacheWindowSize * 2, cacheWindowSize}}
	if len(w.flushed) != len(want) {
		t.Fatalf("got %d writeback calls, want %d: %v", len(w.flushed), len(want), w.flushed)
	}
	for i, c := range want {
		if w.flushed[i] != c {
			t.Fatalf("writeback call %d = %v, want %v", i, w.flushed[i], c)
		}
	}
	// The last window is still being written back when the copy ends; the final
	// fsync of the file covers it.
	if len(w.dropped) != len(want)-1 {
		t.Fatalf("got %d drop calls, want %d: %v", len(w.dropped), len(want)-1, w.dropped)
	}
	for i, c := range want[:len(want)-1] {
		if w.dropped[i] != c {
			t.Fatalf("drop call %d = %v, want %v", i, w.dropped[i], c)
		}
	}
}

func TestCopyBufferedCacheDropUnsupported(t *testing.T) {
	// A target that does not implement the syscalls must not break the copy,
	// and must not be asked again for every window.
	data := bytes.Repeat([]byte{0x0A}, cacheWindowSize*3)

	w := &recordingCacheWriter{flushErr: unix.EOPNOTSUPP}
	if err := copyBuffered(w, bytes.NewReader(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(w.flushed) != 1 {
		t.Fatalf("writeback attempted %d times, want 1 (then give up for good): %v", len(w.flushed), w.flushed)
	}
	if w.written != cacheWindowSize*3 {
		t.Fatalf("wrote %d bytes, want %d", w.written, cacheWindowSize*3)
	}
}

func TestCopyBufferedCacheDropIOError(t *testing.T) {
	// An IO error is not a missing implementation: the kernel reports a
	// writeback failure once, so swallowing it here would let the final fsync
	// report success over data that never reached the storage. Neither is
	// EINVAL, which sync_file_range raises for arguments this code built
	// wrong: taking it for a missing implementation would disable the flush
	// for the whole import and bring the OOM back.
	data := bytes.Repeat([]byte{0x0B}, cacheWindowSize*3)

	for name, tc := range map[string]struct {
		w    *recordingCacheWriter
		want error
	}{
		"writeback fails":                 {&recordingCacheWriter{flushErr: unix.EIO}, unix.EIO},
		"drop fails":                      {&recordingCacheWriter{dropErr: unix.EIO}, unix.EIO},
		"writeback rejects the arguments": {&recordingCacheWriter{flushErr: unix.EINVAL}, unix.EINVAL},
	} {
		t.Run(name, func(t *testing.T) {
			if err := copyBuffered(tc.w, bytes.NewReader(data)); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}

type recordingCacheWriter struct {
	recordingZeroWriter
	flushed, dropped  []zeroCall
	flushErr, dropErr error
}

func (w *recordingCacheWriter) StartWriteback(start, length int64) error {
	w.flushed = append(w.flushed, zeroCall{start, length})
	return w.flushErr
}

func (w *recordingCacheWriter) DropRangeCache(start, length int64) error {
	w.dropped = append(w.dropped, zeroCall{start, length})
	return w.dropErr
}

type zeroCall struct {
	start  int64
	length int64
}

type recordingZeroWriter struct {
	zeroed  []zeroCall
	written int
	zeroErr error
}

func (w *recordingZeroWriter) Write(p []byte) (int, error) {
	w.written += len(p)
	return len(p), nil
}

func (w *recordingZeroWriter) ZeroRange(start, length int64) error {
	w.zeroed = append(w.zeroed, zeroCall{start, length})
	return w.zeroErr
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

func TestFailureClassification(t *testing.T) {
	// A storage failure must not be reported as a failed download: the message text
	// cannot tell them apart, and the page cache flush adds a new class of them.
	// Three windows: the cache drop only happens once the window after it filled up.
	data := bytes.Repeat([]byte{0x0C}, cacheWindowSize*3)

	writeFailures := map[string]io.Writer{
		"write fails":      failWriter{unix.EIO},
		"writeback fails":  &recordingCacheWriter{flushErr: unix.ENOSPC},
		"cache drop fails": &recordingCacheWriter{dropErr: unix.EIO},
	}
	for name, w := range writeFailures {
		t.Run(name, func(t *testing.T) {
			err := copyBuffered(w, bytes.NewReader(data))
			var writeErr *WriteFailedError
			if !errors.As(err, &writeErr) {
				t.Fatalf("got %T (%v), want *WriteFailedError", err, err)
			}
		})
	}

	t.Run("read failure stays a pull error", func(t *testing.T) {
		errBoom := errors.New("connection reset")
		dest := filepath.Join(t.TempDir(), "dest.img")
		err := streamDataToFile(errReader{errBoom}, dest)
		var pullErr *ImagePullFailedError
		if !errors.As(err, &pullErr) {
			t.Fatalf("got %T (%v), want *ImagePullFailedError", err, err)
		}
	})

	t.Run("write failure survives streamDataToFile", func(t *testing.T) {
		// The whole point: what reaches the caller (and the termination message)
		// says the image could not be stored, not that it could not be pulled.
		var writeErr *WriteFailedError
		err := copyBuffered(failWriter{unix.ENOSPC}, bytes.NewReader(data))
		if !errors.As(err, &writeErr) || !errors.Is(err, unix.ENOSPC) {
			t.Fatalf("got %v, want a WriteFailedError wrapping ENOSPC", err)
		}
	})
}
