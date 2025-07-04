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
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/fuzz"
	"kubevirt.io/containerized-data-importer/pkg/common"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"
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
	f.Add(minimalQCow2[:])

	url := fmt.Sprintf("http://%s:%d/upload", addr, uploaderPort)
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzz.ProcessRequests(t, data, url, http.MethodPost, http.MethodPut)
	})
}

func startUploaderServer(tb testing.TB, addr string, mockPort int) (uploaderPort int) {
	tb.Helper()

	endpoint := fmt.Sprintf("%s:%d/uploader", addr, mockPort)

	if err := os.Setenv(common.UploaderDestinationEndpoint, endpoint); err != nil {
		tb.Fatalf("failed to set env var; %v", err)
	}

	if err := os.Setenv(common.UploaderDestinationAuthConfig, "testdata/auth.json"); err != nil {
		tb.Fatalf("failed to set env var; %v", err)
	}

	uploaderServer, err := NewUploadServer(addr, uploaderPort, "", "", "", "", cryptowatch.CryptoConfig{})
	if err != nil {
		tb.Fatalf("failed to initialize uploader server; %v", err)
	}

	srv := uploaderServer.(*uploadServerApp)
	srv.keepAlive = true
	srv.keepConcurrent = true
	srv.destInsecure = true
	// take free port for uploader server
	srv.bindPort = 0
	// take free port for healthz endpoint
	srv.bindHealthzPort = 0

	go func() {
		if err := uploaderServer.Run(); err != nil {
			tb.Fatalf("failed to run uploader server: %v", err)
		}
	}()

	// wait server for start listening
	<-srv.listenChan

	return srv.bindPort
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
		w.Header().Add("Content-Length", "10")
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
