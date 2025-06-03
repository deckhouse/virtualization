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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"kubevirt.io/containerized-data-importer/pkg/common"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"
)

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
		req := &http.Request{Header: make(http.Header)}
		req.Header["Content-Length"] = []string{contentLength}

		parseHTTPHeader(req)

		if contentLength != "" {
			req.Header.Set("Content-Length", contentLength)
		}
	})
}

func FuzzValidateShouldHandleRequest(f *testing.F) {
	seeds := []struct {
		method     string
		clientName string
		hasTLS     bool
		uploading  bool
	}{
		{http.MethodGet, "test-client", true, false},
		{http.MethodHead, "test-client", true, false},
		{http.MethodPost, "test-client", true, false},
		{http.MethodPut, "test-client", true, false},
		{http.MethodPatch, "test-client", true, false},
		{http.MethodDelete, "test-client", true, false},
		{http.MethodConnect, "test-client", true, false},
		{http.MethodOptions, "test-client", true, false},
		{http.MethodTrace, "test-client", true, false},

		{"POST", "wrong-client", true, false},
		{"POST", "", true, false},
		{"POST", "test-client", false, true},
	}

	for _, seed := range seeds {
		f.Add(seed.method, seed.clientName, seed.hasTLS, seed.uploading)
	}

	f.Fuzz(func(t *testing.T, method, clientName string, hasTLS, uploading bool) {
		server, err := NewUploadServer("127.0.0.1", 0, "", "", "", "test-client", cryptowatch.CryptoConfig{})
		if err != nil {
			t.Fatalf("Failed to create upload server: %v", err)
		}

		app := server.(*uploadServerApp)

		go app.Run()

		w := httptest.NewRecorder()
		req := &http.Request{Header: make(http.Header), Method: method, URL: &url.URL{Path: "/upload"}}

		if hasTLS {
			// Create a mock certificate
			cert := &x509.Certificate{
				Subject: pkix.Name{
					CommonName: clientName,
				},
			}
			req.TLS = &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
			}
		}

		isValid := app.validateShouldHandleRequest(w, req)

		// Log interesting findings
		if isValid && (method != "POST" && method != "PUT") {
			t.Errorf("Unexpected success for method %s", method)
		}
		if isValid && hasTLS && clientName != "test-client" {
			t.Errorf("Unexpected success for wrong client name: %s", clientName)
		}
	})
}

func FuzzNewContentReader(f *testing.F) {
	seeds := []struct {
		data        string
		contentType string
	}{
		{"test data", "application/octet-stream"},
		{"", ""},
		{"compressed data", common.BlockdeviceClone},
		{"json data", "application/json"},
		{"plain text", "text/plain"},
		{"binary\x00\x01\x02\x03", "application/octet-stream"},
		{"large data " + strings.Repeat("x", 1000), "text/plain"},
		{"snappy data", common.BlockdeviceClone},
	}

	for _, seed := range seeds {
		f.Add(seed.data, seed.contentType)
	}

	f.Fuzz(func(t *testing.T, data, contentType string) {
		stream := io.NopCloser(strings.NewReader(data))
		reader := newContentReader(stream, contentType)

		if reader == nil {
			t.Error("newContentReader returned nil")
			return
		}

		// Try to read some data to ensure the reader works
		buffer := make([]byte, 100)
		n, err := reader.Read(buffer)

		// For snappy content, read errors are expected for non-snappy data
		if contentType == common.BlockdeviceClone && err != nil {
			t.Logf("Snappy read error (expected for non-snappy data): %v", err)
		} else if err != nil && err != io.EOF {
			t.Logf("Read error: %v", err)
		}

		if n > len(data) && contentType != common.BlockdeviceClone {
			t.Logf("Read more data than input for non-compressed content: got %d, input %d", n, len(data))
		}

		// Always close the reader
		if err := reader.Close(); err != nil {
			t.Logf("Close error: %v", err)
		}
	})
}
