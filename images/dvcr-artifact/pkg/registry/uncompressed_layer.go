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
	"crypto"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"os"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

var (
	// errNotComputed is returned when the requested value is not yet computed
	// because the stream has not been consumed yet.
	errNotComputed = errors.New("value not computed until stream is consumed")

	// errConsumed is returned when the underlying stream has already been
	// consumed and closed.
	errConsumed = errors.New("stream was already consumed")
)

// uncompressedLayer is a single-pass streaming v1.Layer that uploads the raw
// (uncompressed) tar stream as an application/vnd.docker.image.rootfs.diff.tar
// layer.
//
// It mirrors go-containerregistry's stream.Layer but skips gzip compression.
// gzip (gzip.BestSpeed, single goroutine) is the CPU bottleneck of the importer
// upload path: the provisioning pod is CPU-limited, so compressing a multi-GB
// disk image caps the whole pipeline at a few MB/s. Disk images barely compress
// anyway, so an uncompressed layer is roughly the same size with near-zero CPU.
//
// For an uncompressed layer the on-disk blob equals the uncompressed content,
// so Digest and DiffID are identical (both the sha256 of the raw tar stream).
type uncompressedLayer struct {
	blob     io.ReadCloser
	consumed bool

	mu     sync.Mutex
	digest *v1.Hash
	size   int64
}

var _ v1.Layer = (*uncompressedLayer)(nil)

// newUncompressedLayer creates an uncompressed streaming Layer from rc.
func newUncompressedLayer(rc io.ReadCloser) *uncompressedLayer {
	return &uncompressedLayer{blob: rc}
}

// Digest implements v1.Layer. It returns errNotComputed until the stream is
// consumed, which marks the layer as streaming for remote.WriteLayer.
func (l *uncompressedLayer) Digest() (v1.Hash, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.digest == nil {
		return v1.Hash{}, errNotComputed
	}
	return *l.digest, nil
}

// DiffID implements v1.Layer. For an uncompressed layer it equals Digest.
func (l *uncompressedLayer) DiffID() (v1.Hash, error) {
	return l.Digest()
}

// Size implements v1.Layer.
func (l *uncompressedLayer) Size() (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size == 0 {
		return 0, errNotComputed
	}
	return l.size, nil
}

// MediaType implements v1.Layer.
func (l *uncompressedLayer) MediaType() (types.MediaType, error) {
	return types.DockerUncompressedLayer, nil
}

// Uncompressed implements v1.Layer.
func (l *uncompressedLayer) Uncompressed() (io.ReadCloser, error) {
	return l.reader()
}

// Compressed implements v1.Layer. The layer is not compressed, so this returns
// the raw tar stream unchanged.
func (l *uncompressedLayer) Compressed() (io.ReadCloser, error) {
	return l.reader()
}

func (l *uncompressedLayer) reader() (io.ReadCloser, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.consumed {
		return nil, errConsumed
	}
	return newUncompressedReader(l), nil
}

// finalize sets the layer to consumed and records the digest and size computed
// while streaming.
func (l *uncompressedLayer) finalize(h hash.Hash, size int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	digest, err := v1.NewHash("sha256:" + hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		return err
	}

	l.digest = &digest
	l.size = size
	l.consumed = true
	return nil
}

type uncompressedReader struct {
	pr     io.Reader
	closer func() error
}

func newUncompressedReader(l *uncompressedLayer) *uncompressedReader {
	// Collect the digest and size of the raw stream as it is read.
	h := crypto.SHA256.New()
	count := &countWriter{}

	pr, pw := io.Pipe()

	// Tee the raw blob to the pipe reader (consumed by the uploader), the
	// hasher (digest), and the counter (size).
	mw := io.MultiWriter(pw, h, count)

	doneDigesting := make(chan struct{})

	r := &uncompressedReader{
		pr: pr,
		closer: func() error {
			// NOTE: pw.Close never returns an error.
			_ = pw.Close()

			// Close the inner ReadCloser. net/http may have already closed it
			// on success, so ignore os.ErrClosed.
			if err := l.blob.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				return err
			}

			<-doneDigesting
			return l.finalize(h, count.n)
		},
	}

	go func() {
		_, copyErr := io.Copy(mw, l.blob)
		if copyErr != nil {
			close(doneDigesting)
			pw.CloseWithError(copyErr)
			return
		}

		// Notify closer that digest/size are done being written.
		close(doneDigesting)

		// Close the reader to finalize digest/size. This causes pr to return
		// EOF so readers of the stream finish.
		pw.CloseWithError(r.Close())
	}()

	return r
}

func (r *uncompressedReader) Read(b []byte) (int, error) { return r.pr.Read(b) }

func (r *uncompressedReader) Close() error { return r.closer() }

// countWriter counts bytes written to it.
type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}
