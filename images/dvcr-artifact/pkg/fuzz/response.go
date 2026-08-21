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

package fuzz

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Response is a hostile HTTP response for a mock remote image source: the status
// line, the headers and the body all come from the fuzzer. Everything except a
// redirect is written to the hijacked connection by hand, because the shapes
// worth fuzzing are exactly the ones http.ResponseWriter refuses to emit: a
// negative, garbage or duplicated Content-Length, a body that disagrees with the
// declared length, a chunked stream with a broken size line, a body cut in the
// middle of the transfer.
//
// The input layout is ResponseKnobs leading bytes followed by the body:
//
//	byte 0  status code index    (see responseStatuses)
//	byte 1  Content-Length mode  (see responseContentLength)
//	byte 2  Content-Type index   (see responseContentTypes)
//	byte 3  flag bits            (Flag* constants)
//	byte 4  redirect hops, redirect mode and chunk switch (Route* constants)
//	byte 5+ response body
//
// Missing bytes read as zero, so NewResponse never fails and never has a reason
// to skip a fuzz iteration: an empty input is a valid response too.
type Response struct {
	status        int
	contentType   string
	contentLength string
	body          []byte
	chunked       bool
	brokenChunk   bool
	declareGzip   bool
	truncate      bool
	slow          bool
	acceptRanges  bool
	headFails     bool
	redirectHops  int
	redirectMode  byte

	// maxHop records how many redirects the client under test actually followed.
	maxHop atomic.Int64
}

// ResponseKnobs is the number of leading input bytes that shape the response.
const ResponseKnobs = 5

// Flag bits of the fourth knob byte.
const (
	// FlagGzipBody gzips the body, without announcing it in Content-Encoding,
	// the way the importer detects compression: by the magic bytes.
	FlagGzipBody byte = 1 << iota
	// FlagDeclareGzip announces Content-Encoding: gzip, truthfully or not.
	FlagDeclareGzip
	// FlagChunked sends the body with Transfer-Encoding: chunked.
	FlagChunked
	// FlagTruncate cuts the body in half and closes the connection.
	FlagTruncate
	// FlagSlow splits the body in two writes with a pause in between.
	FlagSlow
	// FlagPad pads the body up to the header size the importer sniffs, so short
	// inputs still reach the image format detection.
	FlagPad
	// FlagAcceptRanges answers Accept-Ranges: bytes instead of none.
	FlagAcceptRanges
	// FlagHeadFails fails the Content-Length probe while the GET still succeeds.
	FlagHeadFails
)

// Bits of the fifth knob byte.
const (
	// RouteHopsMask is the number of redirects before the response body.
	RouteHopsMask byte = 0x0f
	// RouteModeRelative points Location at a path on the same host.
	RouteModeRelative byte = 0x00
	// RouteModeLoop redirects for as long as the client keeps following.
	RouteModeLoop byte = 0x10
	// RouteModeAbsolute points Location at an absolute URL.
	RouteModeAbsolute byte = 0x20
	// RouteModeBadLocation sends a Location header that is not a URL.
	RouteModeBadLocation byte = 0x30
	// RouteBrokenChunk replaces the chunk size lines with garbage.
	RouteBrokenChunk byte = 0x40

	routeModeMask byte = 0x30
)

const (
	// responsePadSize is several times image.MaxExpectedHdrSize, the 512 bytes the
	// importer reads before it can decide on the image format. Format detection
	// makes one such read per header it recognizes, so a padded body has to carry
	// more than one of them to reach the decompression code at all.
	responsePadSize = 2048

	// responseBodyLimit keeps one iteration cheap no matter how large the corpus
	// entry grew.
	responseBodyLimit = 1 << 20

	// maxServedRedirects bounds the redirect loop. A client with a redirect limit
	// gives up long before this; one without it would keep the fuzz target
	// hanging forever, so the mock source stops instead.
	maxServedRedirects = 24

	// responseDelay is the pause of a slow body. Long enough to split the
	// transfer into separate reads, short enough to keep fuzzing fast.
	responseDelay = 5 * time.Millisecond

	// hopParam carries the redirect hop counter across a chain.
	hopParam = "fuzzhop"
)

var responseStatuses = []int{
	http.StatusOK,
	http.StatusOK,
	http.StatusOK,
	http.StatusNoContent,
	http.StatusPartialContent,
	http.StatusMovedPermanently,
	http.StatusFound,
	http.StatusSeeOther,
	http.StatusTemporaryRedirect,
	http.StatusPermanentRedirect,
	http.StatusBadRequest,
	http.StatusUnauthorized,
	http.StatusForbidden,
	http.StatusNotFound,
	http.StatusRequestedRangeNotSatisfiable,
	http.StatusTooManyRequests,
	http.StatusInternalServerError,
	http.StatusBadGateway,
	http.StatusServiceUnavailable,
	599,
}

var responseContentTypes = []string{
	"application/octet-stream",
	"",
	"text/html",
	"text/plain; charset=utf-8",
	"application/x-qemu-disk",
	"application/gzip",
	"application/json",
	// A response header smuggled into the Content-Type value.
	"application/octet-stream\r\nX-Injected: 1",
	strings.Repeat("a", 4096),
	"???",
	"*/*",
	"application/octet-stream; charset=\"\t\"",
}

// NewResponse turns fuzzer bytes into a servable hostile response.
func NewResponse(data []byte) *Response {
	knob := func(i int) byte {
		if i < len(data) {
			return data[i]
		}
		return 0
	}

	flags := knob(3)
	route := knob(4)

	var body []byte
	if len(data) > ResponseKnobs {
		body = data[ResponseKnobs:]
	}
	if len(body) > responseBodyLimit {
		body = body[:responseBodyLimit]
	}
	if flags&FlagPad != 0 {
		body = pad(body)
	}
	if flags&FlagGzipBody != 0 {
		body = gzipBytes(body)
		if flags&FlagPad != 0 {
			// A gzip stream of a padded body is tiny again, and the importer would
			// hit the end of it before it ever looked for a gzip header. Padding
			// past the end of the stream also leaves trailing garbage for the
			// decompressor to trip over.
			body = pad(body)
		}
	}

	return &Response{
		status:        responseStatuses[int(knob(0))%len(responseStatuses)],
		contentType:   responseContentTypes[int(knob(2))%len(responseContentTypes)],
		contentLength: responseContentLength(knob(1), len(body)),
		body:          body,
		chunked:       flags&FlagChunked != 0,
		brokenChunk:   route&RouteBrokenChunk != 0,
		declareGzip:   flags&FlagDeclareGzip != 0,
		truncate:      flags&FlagTruncate != 0,
		slow:          flags&FlagSlow != 0,
		acceptRanges:  flags&FlagAcceptRanges != 0,
		headFails:     flags&FlagHeadFails != 0,
		redirectHops:  int(route & RouteHopsMask),
		redirectMode:  route & routeModeMask,
	}
}

// ServeHTTP answers with the fuzzed response. It never panics and never fails
// the surrounding server, so the same mock source survives any number of
// malformed exchanges.
func (r *Response) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r.serveRedirect(w, req) {
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "connection cannot be hijacked", http.StatusInternalServerError)
		return
	}

	conn, bw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	r.writeRawResponse(bw.Writer, req.Method == http.MethodHead)
}

// FollowedRedirects reports how many redirects of the chain the client followed.
func (r *Response) FollowedRedirects() int {
	return int(r.maxHop.Load())
}

func (r *Response) String() string {
	return fmt.Sprintf(
		"status=%d content-length=%q content-type=%.32q body=%d chunked=%t broken-chunk=%t "+
			"declare-gzip=%t truncate=%t slow=%t accept-ranges=%t head-fails=%t hops=%d mode=%#x",
		r.status, r.contentLength, r.contentType, len(r.body), r.chunked, r.brokenChunk,
		r.declareGzip, r.truncate, r.slow, r.acceptRanges, r.headFails, r.redirectHops, r.redirectMode,
	)
}

func (r *Response) serveRedirect(w http.ResponseWriter, req *http.Request) bool {
	hops := r.redirectHops
	if r.redirectMode == RouteModeLoop && hops > 0 {
		hops = maxServedRedirects
	}

	hop, _ := strconv.Atoi(req.URL.Query().Get(hopParam))
	if hop < 0 || hop >= hops || hop >= maxServedRedirects {
		return false
	}

	if next := int64(hop + 1); next > r.maxHop.Load() {
		r.maxHop.Store(next)
	}

	code := r.status
	if code < http.StatusMultipleChoices || code > http.StatusPermanentRedirect {
		code = http.StatusFound
	}

	target := fmt.Sprintf("%s?%s=%d", req.URL.Path, hopParam, hop+1)
	switch r.redirectMode {
	case RouteModeAbsolute:
		target = "http://" + req.Host + target
	case RouteModeBadLocation:
		target = ":// not a url"
	}

	w.Header().Set("Location", target)
	w.WriteHeader(code)

	return true
}

func (r *Response) writeRawResponse(bw *bufio.Writer, headOnly bool) {
	status := r.status
	if headOnly && r.headFails {
		status = http.StatusInternalServerError
	}

	text := http.StatusText(status)
	if text == "" {
		text = "Fuzzed"
	}

	writef(bw, "HTTP/1.1 %d %s\r\n", status, text)
	if r.contentType != "" {
		writef(bw, "Content-Type: %s\r\n", r.contentType)
	}
	if r.contentLength != "" {
		writef(bw, "Content-Length: %s\r\n", r.contentLength)
	}
	if r.chunked {
		writef(bw, "Transfer-Encoding: chunked\r\n")
	}
	if r.declareGzip {
		writef(bw, "Content-Encoding: gzip\r\n")
	}
	if r.acceptRanges {
		writef(bw, "Accept-Ranges: bytes\r\n")
	} else {
		writef(bw, "Accept-Ranges: none\r\n")
	}
	writef(bw, "Connection: close\r\n\r\n")

	if headOnly {
		flush(bw)
		return
	}

	r.writeBody(bw)
	flush(bw)
}

func (r *Response) writeBody(bw *bufio.Writer) {
	body := r.body
	if r.truncate {
		body = body[:len(body)/2]
	}

	if r.chunked {
		r.writeChunkedBody(bw, body)
		return
	}

	if r.slow && len(body) > 1 {
		half := len(body) / 2
		writeBytes(bw, body[:half])
		flush(bw)
		time.Sleep(responseDelay)
		body = body[half:]
	}

	writeBytes(bw, body)
}

func (r *Response) writeChunkedBody(bw *bufio.Writer, body []byte) {
	const chunks = 3

	size := len(body)/chunks + 1
	for offset := 0; offset < len(body); offset += size {
		part := body[offset:min(offset+size, len(body))]
		if r.brokenChunk {
			// Not a hexadecimal chunk size.
			writef(bw, "zz\r\n")
		} else {
			writef(bw, "%x\r\n", len(part))
		}
		writeBytes(bw, part)
		writef(bw, "\r\n")
		flush(bw)

		if r.slow {
			time.Sleep(responseDelay)
		}
	}

	if r.truncate {
		// No terminating chunk: the client runs into an unexpected EOF.
		return
	}

	writef(bw, "0\r\n\r\n")
}

// responseContentLength returns the raw Content-Length header value, empty when
// the header is left out.
func responseContentLength(mode byte, bodyLen int) string {
	switch mode % 14 {
	case 0:
		return ""
	case 1:
		return strconv.Itoa(bodyLen)
	case 2:
		return "0"
	case 3:
		return "-1"
	case 4:
		// Longer than the body: the client waits for bytes that never arrive.
		return strconv.Itoa(bodyLen + 4096)
	case 5:
		// Shorter than the body: the tail is left on the connection.
		return strconv.Itoa(max(bodyLen-1, 0))
	case 6:
		// One past the uint64 range.
		return "18446744073709551616"
	case 7:
		return "99999999999999999999999999"
	case 8:
		return strconv.FormatInt(1<<62, 10)
	case 9:
		return "abc"
	case 10:
		return " 512 "
	case 11:
		return "+512"
	case 12:
		return "0x200"
	default:
		// Two conflicting Content-Length headers in one response.
		return strconv.Itoa(bodyLen) + "\r\nContent-Length: " + strconv.Itoa(bodyLen+1)
	}
}

// pad grows data up to responsePadSize with zero bytes. The result is always a
// fresh slice: the fuzzer input must not be modified.
func pad(data []byte) []byte {
	if len(data) >= responsePadSize {
		return data
	}

	padded := make([]byte, responsePadSize)
	copy(padded, data)

	return padded
}

func gzipBytes(data []byte) []byte {
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	// bytes.Buffer never fails, and a fuzzed body has no error path anyway.
	_, _ = gz.Write(data)
	_ = gz.Close()

	return buf.Bytes()
}

// The mock source has nobody to report a write error to: the client under test
// is free to hang up at any point of a malformed exchange.
func writef(bw *bufio.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(bw, format, args...)
}

func writeBytes(bw *bufio.Writer, data []byte) {
	_, _ = bw.Write(data)
}

func flush(bw *bufio.Writer) {
	_ = bw.Flush()
}
