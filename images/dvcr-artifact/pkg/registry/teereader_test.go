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
	"strings"
	"testing"
)

// errWriter fails every Write call with a fixed error.
type errWriter struct {
	err     error
	written int
}

func (w *errWriter) Write(p []byte) (int, error) {
	w.written++
	return 0, w.err
}

func TestNonBlockingTeeReader_MirrorsWritesUntilEOF(t *testing.T) {
	src := strings.NewReader("hello world")
	var sink bytes.Buffer

	r := nonBlockingTeeReader(src, &sink)
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: unexpected error: %v", err)
	}
	if string(out) != "hello world" {
		t.Fatalf("read returned %q, want %q", string(out), "hello world")
	}
	if sink.String() != "hello world" {
		t.Fatalf("sink got %q, want %q", sink.String(), "hello world")
	}
}

func TestNonBlockingTeeReader_WriteErrorDoesNotPropagate(t *testing.T) {
	src := strings.NewReader("abcdefghij")
	w := &errWriter{err: io.ErrClosedPipe}

	r := nonBlockingTeeReader(src, w)
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: unexpected error: %v", err)
	}
	if string(out) != "abcdefghij" {
		t.Fatalf("read returned %q, want all input", string(out))
	}
}

func TestNonBlockingTeeReader_StopsWritingAfterFirstWriteError(t *testing.T) {
	// Use a reader that returns data in two chunks so we can observe that
	// after the first write fails, the writer is no longer called.
	src := io.MultiReader(
		strings.NewReader("first-"),
		strings.NewReader("second"),
	)
	w := &errWriter{err: errors.New("boom")}

	r := nonBlockingTeeReader(src, w)

	buf := make([]byte, 6)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if string(buf) != "first-" {
		t.Fatalf("first chunk got %q, want %q", string(buf), "first-")
	}
	if w.written != 1 {
		t.Fatalf("writer should have been called once before failure, got %d", w.written)
	}

	rest, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rest read: %v", err)
	}
	if string(rest) != "second" {
		t.Fatalf("rest got %q, want %q", string(rest), "second")
	}
	if w.written != 1 {
		t.Fatalf("writer must not be called after failure, got %d calls total", w.written)
	}
}
