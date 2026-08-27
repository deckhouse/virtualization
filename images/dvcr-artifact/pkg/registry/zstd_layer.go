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
	"runtime"
	"sync"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/zstd"
)

const (
	// zstdEncoderMaxConcurrency caps the number of encoder goroutines to the CPU
	// limit of the provisioning pod. GOMAXPROCS reflects the node, not the
	// cgroup, so an unbounded encoder would spawn dozens of goroutines only to
	// be throttled.
	zstdEncoderMaxConcurrency = 4
)

// errUncompressedNotSupported is returned by zstdLayer.Uncompressed: the layer
// is push-only and the pusher only ever asks for the compressed form.
var errUncompressedNotSupported = errors.New("uncompressed form is not available for a zstd streaming layer")

// zstdLayer is a single-pass streaming v1.Layer that compresses the tar stream
// with zstd on the way to the registry.
//
// It exists for raw disk images only. Such an image is mostly zeroes — a 32 GiB
// Windows image carries about 72% of them — so it stores and transfers several
// times smaller compressed, while qcow2 already packs its data and gains
// nothing. gzip was measured at 32 MB/s on this data and is the reason
// compression was dropped here before (see uncompressedLayer); zstd level 1
// does the same job at 168 MB/s single-threaded, so it is a different trade.
//
// Unlike uncompressedLayer, the blob and the content differ, so Digest (over
// the compressed blob) and DiffID (over the tar stream) are computed
// separately, in one pass.
type zstdLayer struct {
	blob     io.ReadCloser
	consumed bool

	mu     sync.Mutex
	digest *v1.Hash
	diffID *v1.Hash
	size   int64
}

var _ v1.Layer = (*zstdLayer)(nil)

// newZstdLayer creates a zstd-compressed streaming Layer from rc.
func newZstdLayer(rc io.ReadCloser) *zstdLayer {
	return &zstdLayer{blob: rc}
}

// Digest implements v1.Layer: the sha256 of the compressed blob. Until the
// stream is consumed it returns stream.ErrNotComputed, which is how remote's
// pusher detects a streaming layer and switches to its chunked upload path.
func (l *zstdLayer) Digest() (v1.Hash, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.digest == nil {
		return v1.Hash{}, stream.ErrNotComputed
	}
	return *l.digest, nil
}

// DiffID implements v1.Layer: the sha256 of the uncompressed tar stream.
func (l *zstdLayer) DiffID() (v1.Hash, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.diffID == nil {
		return v1.Hash{}, stream.ErrNotComputed
	}
	return *l.diffID, nil
}

// Size implements v1.Layer: the size of the compressed blob.
func (l *zstdLayer) Size() (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size == 0 {
		return 0, stream.ErrNotComputed
	}
	return l.size, nil
}

// MediaType implements v1.Layer.
func (l *zstdLayer) MediaType() (types.MediaType, error) {
	return types.OCILayerZStd, nil
}

// Compressed implements v1.Layer.
func (l *zstdLayer) Compressed() (io.ReadCloser, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.consumed {
		return nil, stream.ErrConsumed
	}
	return newZstdReader(l)
}

// Uncompressed implements v1.Layer. The layer is push-only and the pusher never
// asks for the uncompressed form, so this is not supported rather than silently
// returning the compressed bytes.
func (l *zstdLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, errUncompressedNotSupported
}

// finalize records the digests and the compressed size collected while
// streaming, and marks the layer consumed.
func (l *zstdLayer) finalize(compressed, uncompressed hash.Hash, size int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	digest, err := v1.NewHash("sha256:" + hex.EncodeToString(compressed.Sum(nil)))
	if err != nil {
		return err
	}
	diffID, err := v1.NewHash("sha256:" + hex.EncodeToString(uncompressed.Sum(nil)))
	if err != nil {
		return err
	}

	l.digest = &digest
	l.diffID = &diffID
	l.size = size
	l.consumed = true
	return nil
}

type zstdReader struct {
	pr     io.Reader
	closer func() error
}

func newZstdReader(l *zstdLayer) (*zstdReader, error) {
	// hCompressed/count see the blob as the registry stores it; hUncompressed
	// sees the tar stream, which is what DiffID must cover.
	hCompressed := crypto.SHA256.New()
	hUncompressed := crypto.SHA256.New()
	count := &countWriter{}

	pr, pw := io.Pipe()

	enc, err := zstd.NewWriter(
		io.MultiWriter(pw, hCompressed, count),
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderConcurrency(min(runtime.GOMAXPROCS(0), zstdEncoderMaxConcurrency)),
	)
	if err != nil {
		return nil, err
	}

	doneCompressing := make(chan struct{})

	r := &zstdReader{
		pr: pr,
		closer: func() error {
			// NOTE: pw.Close never returns an error.
			_ = pw.Close()
			<-doneCompressing
			return l.finalize(hCompressed, hUncompressed, count.n)
		},
	}

	go func() {
		_, copyErr := io.Copy(enc, io.TeeReader(l.blob, hUncompressed))
		if copyErr == nil {
			// Flushing the encoder is what writes the frame epilogue, so its
			// error matters as much as the copy's.
			copyErr = enc.Close()
		} else {
			_ = enc.Close()
		}

		if closeErr := l.blob.Close(); copyErr == nil {
			copyErr = closeErr
		}

		close(doneCompressing)

		if copyErr != nil {
			pw.CloseWithError(copyErr)
			return
		}

		pw.CloseWithError(r.Close())
	}()

	return r, nil
}

func (r *zstdReader) Read(b []byte) (int, error) { return r.pr.Read(b) }

func (r *zstdReader) Close() error { return r.closer() }
