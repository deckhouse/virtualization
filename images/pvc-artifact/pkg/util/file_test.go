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

package util

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestAppendZeroWithTruncate(t *testing.T) {
	// The fallback used when the backend has no ZERO_RANGE: the range must read
	// back as zeroes without the zeroes being written, so the file stays sparse
	// and the following write continues right after the hole.
	f, err := os.Create(filepath.Join(t.TempDir(), "sparse.img"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	const length = 8 << 20
	if err := AppendZeroWithTruncate(f, 0, length); err != nil {
		t.Fatalf("AppendZeroWithTruncate: %v", err)
	}
	if _, err := f.Write([]byte{0x0C}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if want := append(make([]byte, length), 0x0C); !bytes.Equal(got, want) {
		t.Fatalf("file content mismatch: len %d, want %d", len(got), len(want))
	}

	var st unix.Stat_t
	if err := unix.Stat(f.Name(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Blocks*512 >= length {
		t.Fatalf("file is not sparse: %d bytes allocated for a %d-byte hole", st.Blocks*512, length)
	}
}

func TestAppendZeroWithTruncateRefusesGap(t *testing.T) {
	// Appending is only safe at the end of the file: a start offset that does
	// not match must fail instead of silently zeroing the wrong range.
	f, err := os.Create(filepath.Join(t.TempDir(), "gap.img"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := AppendZeroWithTruncate(f, 4096, 4096); err == nil {
		t.Fatal("want an error for a start offset past the end of the file, got nil")
	}
}

func TestIsUnsupportedZeroRange(t *testing.T) {
	// The distinction the copy relies on: a missing implementation is tolerated
	// and the path is disabled, an IO error must fail the import. fallocate
	// answers EINVAL for a mode it cannot apply to the file, so zeroing counts
	// that as unsupported.
	for err, want := range map[error]bool{
		unix.EOPNOTSUPP: true,
		unix.ENOSYS:     true,
		unix.EINVAL:     true,
		unix.ENOTTY:     false,
		unix.EIO:        false,
		unix.ENOSPC:     false,
		os.ErrClosed:    false,
	} {
		if got := IsUnsupportedZeroRange(err); got != want {
			t.Errorf("IsUnsupportedZeroRange(%v) = %v, want %v", err, got, want)
		}
	}
}

func TestIsUnsupportedFlushRejectsEINVAL(t *testing.T) {
	// sync_file_range documents EINVAL only for a bad flag bit or a bad
	// offset/length, that is, for a bug in the arguments built here. Counting
	// it as unsupported would turn the flush off for the whole import and
	// bring the OOM back, so it has to fail the import instead.
	for err, want := range map[error]bool{
		unix.EOPNOTSUPP: true,
		unix.ENOSYS:     true,
		unix.EINVAL:     false,
		unix.EIO:        false,
		unix.ENOSPC:     false,
		os.ErrClosed:    false,
	} {
		if got := IsUnsupportedFlush(err); got != want {
			t.Errorf("IsUnsupportedFlush(%v) = %v, want %v", err, got, want)
		}
	}
}

func TestStartWritebackAndDropRangeCache(t *testing.T) {
	// Both syscalls must accept a plain file: they run for every window of
	// every import, so a wrong argument order would break every copy.
	f, err := os.Create(filepath.Join(t.TempDir(), "flush.img"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	data := bytes.Repeat([]byte{0x0D}, 1<<20)
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := StartWriteback(f, 0, int64(len(data))); err != nil {
		t.Fatalf("StartWriteback: %v", err)
	}
	if err := DropRangeCache(f, 0, int64(len(data))); err != nil {
		t.Fatalf("DropRangeCache: %v", err)
	}

	got, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("file content changed after flushing the page cache")
	}
}
