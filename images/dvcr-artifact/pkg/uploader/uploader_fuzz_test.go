/*
Copyright 2025 Flant JSC

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
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/fuzz"
)

const (
	addr = "127.0.0.1"
)

func FuzzUploader(f *testing.F) {
	mockPort := startDVCRMockServer(f, addr)
	uploaderPort := startUploaderServer(f, addr, mockPort)

	// 512 bytes is the minimum size of a qcow2 image
	minimalQCow2 := [512]byte{
		0x51, 0x46, 0x49, 0xfb, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00,
	}

	// The body doubles as the source of the fuzzed request headers, so the seeds
	// cover both image formats the uploader detects and header-shaped payloads.
	f.Add(minimalQCow2[:])
	f.Add([]byte(""))
	f.Add([]byte("QFI\xfb"))
	f.Add([]byte("QFI\xfb\x00\x00\x00\x03" + strings.Repeat("\xff", 64)))
	f.Add([]byte("QFI\xfb\x00\x00\x00\x02" + strings.Repeat("\x00", 504)))
	f.Add(bytes.Repeat([]byte{0x00}, 1024))
	f.Add(bytes.Repeat([]byte{0xff}, 1024))
	f.Add([]byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03"))
	f.Add([]byte("\xff\x06\x00\x00sNaPpY"))
	f.Add([]byte("# Disk DescriptorFile\nversion=1\nCID=fffffffe\n"))
	f.Add([]byte("KDMV\x01\x00\x00\x00"))
	f.Add([]byte("conectix"))
	f.Add([]byte("vhdxfile"))
	f.Add([]byte("CD001"))
	f.Add([]byte(`{"schemaVersion":2,"layers":[]}`))
	f.Add([]byte("Content-Length: -1\r\nContent-Type: application/octet-stream\r\n\r\n"))
	f.Add([]byte("Content-Length: 99999999999999999999\r\n\r\n"))
	f.Add([]byte("Transfer-Encoding: chunked\r\n\r\n5\r\nabcde\r\n0\r\n\r\n"))
	f.Add([]byte("a\r\nX-Injected: 1\r\n"))
	f.Add([]byte("../../etc/passwd"))
	f.Add([]byte(strings.Repeat("A", 8192)))
	f.Add([]byte("\xff\xfe\xfd\xfc"))
	f.Add([]byte("имя-диска"))

	url := fmt.Sprintf("http://%s:%d/upload", addr, uploaderPort)
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzz.ProcessRequests(t, data, url, http.MethodPost, http.MethodPut)
	})
}

func startUploaderServer(tb testing.TB, addr string, mockPort int) (uploaderPort int) {
	tb.Helper()

	endpoint := fmt.Sprintf("%s:%d/uploader", addr, mockPort)

	o := &Options{
		ListenAddress:         addr,
		ListenPort:            0, // take a free port for the uploader server
		DestinationEndpoint:   endpoint,
		DestinationAuthConfig: "testdata/auth.json",
		DestinationInsecure:   true,
	}

	srv, err := o.Complete()
	if err != nil {
		tb.Fatalf("failed to initialize uploader server; %v", err)
	}

	srv.keepAlive = true
	srv.keepConcurrent = true
	// take a free port for the healthz endpoint
	srv.healthzPort = 0

	go func() {
		if err := srv.Run(); err != nil {
			tb.Fatalf("failed to run uploader server: %v", err)
		}
	}()

	// wait server for start listening
	<-srv.startListeningChan

	return srv.boundPort
}

func startDVCRMockServer(tb testing.TB, addr string) (port int) {
	tb.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /v2/uploader/blobs/uploads/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Location", "/v2/uploader/blobs/uploads/test_data")
		w.WriteHeader(http.StatusAccepted)
	})

	mux.HandleFunc("PATCH /v2/uploader/blobs/uploads/{id}/", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Add("Location", fmt.Sprintf("/v2/uploader/blobs/uploads/%s", id))
		w.WriteHeader(http.StatusAccepted)
	})

	mux.HandleFunc("PUT /v2/uploader/blobs/uploads/{id}/", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Add("Location", fmt.Sprintf("/v2/uploader/blobs/uploads/%s", id))
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /v2/uploader/blobs/uploads/{id}/", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Add("Location", fmt.Sprintf("/v2/uploader/blobs/uploads/%s", id))
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("HEAD /v2/uploader/manifests/latest/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/octet-stream")
		// 10 is random value
		w.Header().Add("Content-Length", "10")
		// random digest
		w.Header().Add("Docker-Content-Digest", "sha256:af3ca10a606165f3cad5226c504cea77b9f5169df6a536b26aeffd2e651c0ada")
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("PUT /v2/uploader/manifests/latest/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /v2/uploader/manifests/latest/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /v2/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", addr))
	if err != nil {
		tb.Fatalf("failed to listen: %v", err)
	}

	port = listener.Addr().(*net.TCPAddr).Port

	go func() {
		if err := http.Serve(listener, mux); err != nil {
			tb.Fatalf("failed to serve mock server: %v", err)
		}
	}()

	return port
}
