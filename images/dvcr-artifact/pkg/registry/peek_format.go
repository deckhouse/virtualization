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

	"kubevirt.io/containerized-data-importer/pkg/image"
)

// peekRawImage reports whether the stream is a raw disk image, and returns a
// reader that still yields the bytes it had to consume to tell.
//
// "Raw" here means "no known header matched": qcow2, vmdk and the compressed
// and archive formats all announce themselves in the first bytes, and each of
// them already stores its data packed. Whatever is left is an unpacked image,
// which is the only kind worth compressing on the way to the registry.
//
// A stream shorter than the header window is fine: it is read to the end and
// classified on what came in.
func peekRawImage(rc io.ReadCloser) (bool, io.ReadCloser, error) {
	// Header.Match slices at fixed offsets and panics on a short buffer, so the
	// window handed to it is always full length; only the bytes actually read
	// go back into the stream.
	window := make([]byte, image.MaxExpectedHdrSize)
	n, err := io.ReadFull(rc, window)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, nil, err
	}

	restored := &headerRestoringReader{
		reader: io.MultiReader(bytes.NewReader(window[:n]), rc),
		closer: rc,
	}

	for _, hdr := range image.CopyKnownHdrs() {
		if hdr.Match(window) {
			return false, restored, nil
		}
	}

	return true, restored, nil
}

// headerRestoringReader hands out the peeked header again before the rest of
// the stream, and closes the original source.
type headerRestoringReader struct {
	reader io.Reader
	closer io.Closer
}

func (r *headerRestoringReader) Read(p []byte) (int, error) { return r.reader.Read(p) }

func (r *headerRestoringReader) Close() error { return r.closer.Close() }
