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
	"io"
	"sync/atomic"
)

// nonBlockingTeeReader returns an io.Reader that copies what it reads from r
// into w, but, unlike io.TeeReader, never lets a slow or failed w block or
// fail the read side.
//
// As soon as a write to w returns an error (typically io.ErrClosedPipe once
// the inspect side has finished and closed its end of the pipe), the writer
// is marked done and all subsequent reads bypass w entirely. The error from w
// is intentionally discarded: w is best-effort and must not propagate failures
// to the main upload pipeline.
func nonBlockingTeeReader(r io.Reader, w io.Writer) io.Reader {
	return &nonBlockingTee{r: r, w: w}
}

type nonBlockingTee struct {
	r     io.Reader
	w     io.Writer
	wDone atomic.Bool
}

func (t *nonBlockingTee) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 && !t.wDone.Load() {
		if _, werr := t.w.Write(p[:n]); werr != nil {
			t.wDone.Store(true)
		}
	}
	return n, err
}
