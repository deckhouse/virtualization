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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cc "kubevirt.io/containerized-data-importer/pkg/controller/common"
	"kubevirt.io/containerized-data-importer/pkg/importer"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/fuzz"
)

// Seeds carrying non-ASCII text are written as \x byte escapes rather than as
// literal characters, so this file stays ASCII-only and the CI linters that
// reject non-ASCII source have nothing to flag. The escapes encode the same
// UTF-8 bytes; do not "simplify" them back into literals.

const (
	// sourcePath is the path of the image on the mock remote source.
	sourcePath = "/disk.img"

	// drainLimit bounds how much of the image the target reads. A gzip bomb from
	// a hostile source decompresses without end, and the importer is expected to
	// hand out a plain reader, not to protect the caller from that.
	drainLimit = 8 << 20

	// sourceRedirectLimit is the redirect chain length above which the importer
	// is considered to follow redirects without a limit.
	sourceRedirectLimit = 10

	// headerSize is the amount of bytes the importer reads before it can decide
	// on the image format.
	headerSize = 512

	// fuzzMaxInputSize bounds an iteration: the input carries the response knobs
	// plus the body the mock source serves, and a longer body only makes the
	// transfer slower.
	fuzzMaxInputSize = 1 << 20
)

// FuzzImporterHTTPSource fuzzes what a malicious remote image source answers to
// the importer. The URL of a VirtualImage or ClusterVirtualImage
// spec.dataSource.http.url is attacker controlled, so every byte of the response
// behind it is untrusted: the status line, the headers, the framing and the body.
//
// The target drives the real HTTP data source of pkg/importer against a mock
// source that answers with fuzzer-shaped responses, then consumes it the way
// registry.DataProcessor does - filename, length, reader, format detection,
// stream - and expects errors, not panics, hangs or unbounded reads.
func FuzzImporterHTTPSource(f *testing.F) {
	// f.Fatalf is forbidden inside the fuzz target, so the mock source reports
	// its failures over a channel instead of calling it from its own goroutine.
	srvErrCh := make(chan error, 1)

	var current atomic.Pointer[fuzz.Response]

	srcURL := startFuzzImageSource(f, &current, srvErrCh)

	// 512 bytes is the minimum size of a qcow2 image.
	qcow2 := [headerSize]byte{
		0x51, 0x46, 0x49, 0xfb, 0x00, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10,
	}
	htmlErrorPage := []byte("<html><head><title>404</title></head><body>not here</body></html>")

	// Bytes that do not compress, so a gzip stream of them stays long enough for
	// the format detection to read a header out of it. A fixed xorshift keeps the
	// seed reproducible.
	noise := make([]byte, 8*headerSize)
	state := uint32(0x12345678)
	for i := range noise {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		noise[i] = byte(state)
	}

	// Seeds spell out the five response knobs documented by fuzz.NewResponse:
	// status index, Content-Length mode, Content-Type index, flags, route.
	f.Add(seed(0, 1, 0, fuzz.FlagAcceptRanges, 0, qcow2[:]))                                                     // the honest case: 200, exact length, a real qcow2 header
	f.Add(seed(0, 0, 0, fuzz.FlagPad, 0, []byte("QFI\xfb")))                                                     // no Content-Length at all, body ends with the connection
	f.Add(seed(0, 2, 0, fuzz.FlagPad, 0, qcow2[:]))                                                              // Content-Length: 0 with a body attached
	f.Add(seed(0, 3, 0, fuzz.FlagPad, 0, qcow2[:]))                                                              // Content-Length: -1
	f.Add(seed(0, 4, 0, fuzz.FlagPad, 0, qcow2[:]))                                                              // Content-Length longer than the body: the read never completes
	f.Add(seed(0, 5, 0, fuzz.FlagPad, 0, qcow2[:]))                                                              // Content-Length shorter than the body
	f.Add(seed(0, 6, 0, fuzz.FlagPad, 0, qcow2[:]))                                                              // Content-Length past the uint64 range
	f.Add(seed(0, 7, 0, fuzz.FlagPad, 0, qcow2[:]))                                                              // Content-Length of 26 digits
	f.Add(seed(0, 8, 4, fuzz.FlagPad|fuzz.FlagAcceptRanges, 0, qcow2[:]))                                        // 4 EiB claimed for a 512 byte image
	f.Add(seed(0, 9, 0, fuzz.FlagPad, 0, qcow2[:]))                                                              // Content-Length: abc
	f.Add(seed(0, 10, 0, fuzz.FlagPad, 0, qcow2[:]))                                                             // Content-Length padded with spaces
	f.Add(seed(0, 12, 0, fuzz.FlagPad, 0, qcow2[:]))                                                             // Content-Length: 0x200
	f.Add(seed(0, 13, 0, fuzz.FlagPad, 0, qcow2[:]))                                                             // two conflicting Content-Length headers
	f.Add(seed(0, 0, 0, fuzz.FlagChunked|fuzz.FlagPad, 0, qcow2[:]))                                             // a well formed chunked stream
	f.Add(seed(0, 1, 0, fuzz.FlagChunked|fuzz.FlagPad, fuzz.RouteBrokenChunk, qcow2[:]))                         // chunk sizes that are not hexadecimal
	f.Add(seed(0, 0, 0, fuzz.FlagChunked|fuzz.FlagTruncate|fuzz.FlagPad, 0, qcow2[:]))                           // chunked stream without a terminating chunk
	f.Add(seed(0, 1, 0, fuzz.FlagTruncate|fuzz.FlagPad, 0, qcow2[:]))                                            // the promised length, half the body, then a hangup
	f.Add(seed(0, 1, 5, fuzz.FlagGzipBody|fuzz.FlagPad, 0, qcow2[:]))                                            // a gzip stream the importer has to unwrap
	f.Add(seed(0, 1, 5, fuzz.FlagGzipBody|fuzz.FlagTruncate|fuzz.FlagPad, 0, qcow2[:]))                          // a gzip stream cut in half
	f.Add(seed(0, 0, 0, fuzz.FlagGzipBody|fuzz.FlagDeclareGzip|fuzz.FlagPad, 0, qcow2[:]))                       // gzip announced in Content-Encoding as well
	f.Add(seed(0, 1, 0, fuzz.FlagDeclareGzip|fuzz.FlagPad, 0, qcow2[:]))                                         // Content-Encoding: gzip over plain bytes
	f.Add(seed(0, 1, 5, fuzz.FlagGzipBody, 0, noise))                                                            // a gzip stream long enough to be detected
	f.Add(seed(0, 1, 5, fuzz.FlagGzipBody|fuzz.FlagTruncate, 0, noise))                                          // that stream, cut in half mid-transfer
	f.Add(seed(0, 0, 0, 0, 0, []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03")))                               // a gzip header with nothing behind it
	f.Add(seed(0, 1, 0, fuzz.FlagPad, 0, []byte("\xfd7zXZ\x00")))                                                // an xz header with nothing behind it
	f.Add(seed(0, 1, 0, fuzz.FlagPad, 0, []byte("\x28\xb5\x2f\xfd")))                                            // a zstd header with nothing behind it
	f.Add(seed(0, 1, 0, fuzz.FlagPad, 0, []byte("KDMV")))                                                        // a vmdk header
	f.Add(seed(0, 1, 0, fuzz.FlagPad, 0, []byte("conectix")))                                                    // a vhd header
	f.Add(seed(0, 1, 0, fuzz.FlagPad, 0, []byte("vhdxfile")))                                                    // a vhdx header
	f.Add(seed(0, 1, 2, fuzz.FlagPad, 0, htmlErrorPage))                                                         // an html error page served as the image
	f.Add(seed(13, 1, 2, 0, 0, htmlErrorPage))                                                                   // 404 with the same page
	f.Add(seed(16, 1, 6, 0, 0, []byte(`{"error":"nope"}`)))                                                      // 500 with a json body
	f.Add(seed(19, 1, 0, fuzz.FlagPad, 0, qcow2[:]))                                                             // an unassigned status code
	f.Add(seed(3, 1, 0, fuzz.FlagPad, 0, qcow2[:]))                                                              // 204 with a body anyway
	f.Add(seed(4, 1, 0, fuzz.FlagPad|fuzz.FlagAcceptRanges, 0, qcow2[:]))                                        // 206 without the range being asked for
	f.Add(seed(0, 1, 7, fuzz.FlagPad, 0, qcow2[:]))                                                              // a header smuggled into the Content-Type value
	f.Add(seed(0, 1, 8, fuzz.FlagPad, 0, qcow2[:]))                                                              // a 4 KiB Content-Type value
	f.Add(seed(6, 1, 0, fuzz.FlagPad, 3, qcow2[:]))                                                              // a chain of three redirects
	f.Add(seed(9, 1, 0, fuzz.FlagPad, 2|fuzz.RouteModeAbsolute, qcow2[:]))                                       // 308 redirects to absolute urls
	f.Add(seed(6, 1, 0, fuzz.FlagPad, 1|fuzz.RouteModeBadLocation, qcow2[:]))                                    // a Location header that is not a url
	f.Add(seed(6, 1, 0, fuzz.FlagPad, 1|fuzz.RouteModeLoop, qcow2[:]))                                           // a redirect loop
	f.Add(seed(0, 1, 0, fuzz.FlagHeadFails|fuzz.FlagPad, 0, qcow2[:]))                                           // the length probe fails, the download works
	f.Add(seed(0, 4, 0, fuzz.FlagSlow|fuzz.FlagPad, 0, qcow2[:]))                                                // a slow body that stops halfway
	f.Add(seed(0, 1, 1, fuzz.FlagPad, 0, bytes.Repeat([]byte{0xff}, 1024)))                                      // no Content-Type, unrecognizable bytes
	f.Add([]byte{})                                                                                              // no knobs at all
	f.Add([]byte{0x00})                                                                                          // a single knob byte
	f.Add([]byte("Content-Length: -1\r\n\r\n\xd0\xb8\xd0\xbc\xd1\x8f-\xd0\xb4\xd0\xb8\xd1\x81\xd0\xba\xd0\xb0")) // response headers as the body

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzMaxInputSize {
			t.Skip()
		}

		select {
		case err := <-srvErrCh:
			t.Fatalf("mock image source stopped: %v", err)
		default:
		}

		response := fuzz.NewResponse(data)
		current.Store(response)

		imp := &Importer{
			src:            srcURL,
			srcType:        cc.SourceHTTP,
			srcContentType: string(cdiv1.DataVolumeKubeVirt),
		}

		ds, err := imp.newDataSource(t.Context())
		if err != nil {
			// A hostile source is the normal case: rejecting it is the point.
			t.Logf("data source rejected [%s]: %v", response, err)
			checkRedirects(t, response)

			return
		}
		defer func() {
			if err := ds.Close(); err != nil {
				t.Logf("closing the data source: %v", err)
			}
		}()

		checkRedirects(t, response)

		if _, err := ds.Filename(); err != nil {
			t.Logf("filename [%s]: %v", response, err)
		}

		length, err := ds.Length()
		if err != nil {
			t.Logf("length [%s]: %v", response, err)
		}
		if length < 0 {
			t.Fatalf("negative source length %d [%s]", length, response)
		}

		reader, err := ds.ReadCloser()
		if err != nil {
			t.Logf("reader [%s]: %v", response, err)
			return
		}

		// pkg/registry streams the source through the format readers to detect the
		// image format, so the compression headers of the response end up here.
		readers, err := importer.NewFormatReaders(reader, uint64(length))
		if err != nil {
			t.Logf("format readers [%s]: %v", response, err)
			return
		}
		defer func() {
			if err := readers.Close(); err != nil {
				t.Logf("closing the format readers: %v", err)
			}
		}()

		read, err := io.Copy(io.Discard, io.LimitReader(readers.TopReader(), drainLimit))
		switch {
		case err != nil:
			t.Logf("streaming [%s]: %v", response, err)
		case read == drainLimit:
			t.Logf("source did not end within %d bytes [%s]", drainLimit, response)
		case read != int64(length) && !readers.Archived && !readers.Convert:
			t.Logf("streamed %d bytes of the %d announced [%s]", read, length, response)
		}
	})
}

// checkRedirects reports a source that walked the importer through more
// redirects than any HTTP client should follow.
func checkRedirects(t *testing.T, response *fuzz.Response) {
	t.Helper()

	if hops := response.FollowedRedirects(); hops > sourceRedirectLimit {
		t.Logf("importer followed %d redirects, more than the %d limit of net/http [%s]",
			hops, sourceRedirectLimit, response)
	}
}

// seed builds a fuzz input out of the response knobs and the body.
func seed(status, contentLength, contentType, flags, route byte, body []byte) []byte {
	return append([]byte{status, contentLength, contentType, flags, route}, body...)
}

// startFuzzImageSource runs the mock remote image source. It serves whatever
// response the fuzz target stored last, and stays alive through malformed
// exchanges, hangups and hijacked connections.
func startFuzzImageSource(tb testing.TB, current *atomic.Pointer[fuzz.Response], errCh chan<- error) string {
	tb.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("failed to listen: %v", err)
	}

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := current.Load()
			if response == nil {
				http.Error(w, "no fuzzed response yet", http.StatusServiceUnavailable)
				return
			}

			response.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("mock image source: %w", err)
		}
	}()

	return fmt.Sprintf("http://%s%s", listener.Addr().String(), sourcePath)
}
