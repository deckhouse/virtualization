/*
Copyright 2024 Flant JSC

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

package uploader

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"kubevirt.io/containerized-data-importer/pkg/common"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"
)

type mockReadCloser struct {
	data   []byte
	pos    int
	closed bool
}

func (m *mockReadCloser) Read(p []byte) (int, error) {
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}
	n := copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

type mockResponseWriter struct {
	statusCode int
	headers    http.Header
	body       []byte
}

func (m *mockResponseWriter) Header() http.Header {
	if m.headers == nil {
		m.headers = make(http.Header)
	}
	return m.headers
}

func (m *mockResponseWriter) Write(data []byte) (int, error) {
	m.body = append(m.body, data...)
	return len(data), nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

func FuzzParseHTTPHeader(f *testing.F) {
	seeds := []string{
		"", "0", "1", "1024", "-1", "abc", "123abc",
		"18446744073709551615", // max uint64
		"9223372036854775807",  // max int64
		"   123   ", "\n123", "123\n", "123\r\n",
		"999999999999999999999999999999", // very large number
		"1.5", "1e10", "+123", "0x123",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, contentLength string) {
		server, err := NewUploadServer("127.0.0.1", 0, "", "", "", "", cryptowatch.CryptoConfig{})
		if err != nil {
			t.Fatalf("Failed to create upload server: %v", err)
		}

		app := server.(*uploadServerApp)

		req, err := http.NewRequest("POST", "/upload", strings.NewReader("test data"))
		if err != nil {
			t.Fatalf("Failed to create HTTP request: %v", err)
		}

		if contentLength != "" {
			req.Header.Set("Content-Length", contentLength)
		}

		rw := &mockResponseWriter{
			statusCode: 200,
			headers:    make(http.Header),
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("HTTP handler panicked with Content-Length %q: %v", contentLength, r)
				}
			}()
			app.ServeHTTP(rw, req)
		}()

		if strings.Contains(contentLength, "\n") || strings.Contains(contentLength, "\r") {
			t.Logf("Testing with newline characters: %q -> status: %d", contentLength, rw.statusCode)
		}

		if len(contentLength) > 20 {
			t.Logf("Testing with very long input: %q -> status: %d", contentLength, rw.statusCode)
		}

		if contentLength != "" && contentLength != strings.TrimSpace(contentLength) {
			t.Logf("Testing with whitespace: %q -> status: %d", contentLength, rw.statusCode)
		}
	})
}

func FuzzNewContentReader(f *testing.F) {
	seeds := []string{
		"", "application/octet-stream", "text/plain", "image/jpeg", "application/json",
		common.BlockdeviceClone,
		"BLOCKDEVICE-CLONE",
		"blockdevice-clone\n", "blockdevice-clone ", " blockdevice-clone",
		"blockdevice-clone-other", "other-blockdevice-clone",
		"application/x-blockdevice-clone", "multipart/form-data",
		"application/gzip", "\x00\x01\x02",
		"very-long-content-type-that-exceeds-normal-expectations",
		"content/type;charset=utf-8", "content\ntype", "content\rtype", "content\ttype",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, contentType string) {
		server, err := NewUploadServer("127.0.0.1", 0, "", "", "", "", cryptowatch.CryptoConfig{})
		if err != nil {
			t.Fatalf("Failed to create upload server: %v", err)
		}

		app := server.(*uploadServerApp)

		req, err := http.NewRequest("POST", "/upload", strings.NewReader("test data for fuzzing"))
		if err != nil {
			t.Fatalf("Failed to create HTTP request: %v", err)
		}

		if contentType != "" {
			req.Header.Set(common.UploadContentTypeHeader, contentType)
		}
		req.Header.Set("Content-Length", "20")

		rw := &mockResponseWriter{
			statusCode: 200,
			headers:    make(http.Header),
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("HTTP handler panicked with contentType %q: %v", contentType, r)
				}
			}()
			app.ServeHTTP(rw, req)
		}()

		if len(contentType) > 50 {
			t.Logf("Testing with very long content type: %q -> status: %d", contentType, rw.statusCode)
		}

		if strings.Contains(contentType, "\n") || strings.Contains(contentType, "\r") || strings.Contains(contentType, "\t") {
			t.Logf("Testing content type with control characters: %q -> status: %d", contentType, rw.statusCode)
		}

		if strings.Contains(contentType, "blockdevice") || contentType == common.BlockdeviceClone {
			t.Logf("Testing blockdevice-related content type: %q -> status: %d", contentType, rw.statusCode)
		}
	})
}

func FuzzValidateHTTPRequest(f *testing.F) {
	seeds := []string{
		"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE", "CONNECT",
		"", "post", "PUT ", "INVALID_METHOD", "POST\n", "POST\r\n",
		"really-long-method-name-that-exceeds-reasonable-limits",
		"M€THOD", "METH OD", "\x00METHOD", "METHOD\x00",
		"123POST", "POST123", "PO ST", "P\nOST",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, method string) {
		server, err := NewUploadServer("127.0.0.1", 0, "", "", "", "", cryptowatch.CryptoConfig{})
		if err != nil {
			t.Fatalf("Failed to create upload server: %v", err)
		}

		app := server.(*uploadServerApp)

		req, err := http.NewRequest(method, "http://example.com/upload", nil)
		if err != nil {
			t.Logf("Invalid HTTP method %q: %v", method, err)
			return
		}

		rw := &mockResponseWriter{
			statusCode: 200,
			headers:    make(http.Header),
		}

		var result bool
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("validateShouldHandleRequest panicked with method %q: %v", method, r)
				}
			}()
			result = app.validateShouldHandleRequest(rw, req)
		}()

		if method == "POST" || method == "PUT" {
			if !result && rw.statusCode == 200 {
				t.Errorf("Expected valid method %q to return true, got false", method)
			}
		} else if method != "" {
			if result {
				t.Errorf("Expected invalid method %q to return false, got true", method)
			}
			if rw.statusCode != 404 && method != "POST" && method != "PUT" {
				t.Logf("Method %q resulted in status code %d", method, rw.statusCode)
			}
		}

		if strings.Contains(method, "\n") || strings.Contains(method, "\r") {
			t.Logf("Testing method with control characters: %q -> result: %v, status: %d",
				method, result, rw.statusCode)
		}

		if len(method) > 20 {
			t.Logf("Testing very long method: %q -> result: %v, status: %d",
				method, result, rw.statusCode)
		}

		if method != strings.ToUpper(method) && method != "" {
			t.Logf("Testing non-uppercase method: %q -> result: %v, status: %d",
				method, result, rw.statusCode)
		}
	})
}

func FuzzSnappyDecompression(f *testing.F) {
	seeds := [][]byte{
		{0x00, 0x00, 0x00, 0x00},             // null bytes
		{0xff, 0xff, 0xff, 0xff},             // max bytes
		[]byte("hello world"),                // simple text
		{0x01, 0x02, 0x03, 0x04, 0x05},       // sequential bytes
		{0x73, 0x4e, 0x61, 0x50, 0x70, 0x59}, // "sNaPpY" - snappy magic bytes variant
		{0x00, 0x01, 0x00, 0x00},             // small compressed-like sequence
		make([]byte, 1000),                   // large empty buffer
		{0x01, 0x00, 0x00, 0x00, 0x01},       // potential snappy frame
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		mockStream := &mockReadCloser{
			data: data,
			pos:  0,
		}

		var snappyReader io.ReadCloser
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("newSnappyReadCloser panicked with data length %d: %v", len(data), r)
				}
			}()
			snappyReader = newSnappyReadCloser(mockStream)
		}()

		if snappyReader == nil {
			t.Fatal("newSnappyReadCloser returned nil")
		}

		buffer := make([]byte, 1024)
		var totalRead int
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("snappy reader panicked during read with data length %d: %v", len(data), r)
					}
				}()
				n, err := snappyReader.Read(buffer)
				totalRead += n
				if err == io.EOF {
					return
				}
				if err != nil && err != io.EOF {

					t.Logf("Snappy decompression error for data length %d: %v", len(data), err)
					return
				}
			}()
			break
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("snappy reader panicked during close: %v", r)
				}
			}()
			if err := snappyReader.Close(); err != nil {
				t.Logf("Error closing snappy reader: %v", err)
			}
		}()

		if len(data) > 0 && totalRead == 0 {
			t.Logf("No data read from snappy reader with input length %d", len(data))
		}

		if totalRead > len(data)*2 {
			t.Logf("Snappy decompression expanded data significantly: %d -> %d", len(data), totalRead)
		}

		if len(data) > 100 {
			t.Logf("Testing large input: %d bytes -> %d bytes output", len(data), totalRead)
		}

		mockStream2 := &mockReadCloser{
			data: data,
			pos:  0,
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("newContentReader panicked with blockdevice-clone content type: %v", r)
				}
			}()
			contentReader := newContentReader(mockStream2, common.BlockdeviceClone)
			if contentReader == nil {
				t.Error("newContentReader returned nil for blockdevice-clone content type")
			} else {
				contentReader.Close()
			}
		}()
	})
}
