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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/fuzz"
	"kubevirt.io/containerized-data-importer/pkg/common"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"
)

const (
	addr = "127.0.0.1"
)

func FuzzUploader(f *testing.F) {
	mockURL := startDVCRMockServer(f)
	uploaderURL := startUploaderServer(f, mockURL)

	// 512 bytes is the minimum size of a qcow2 image
	minimalQCow2 := [512]byte{
		0x51, 0x46, 0x49, 0xfb, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00,
	}
	f.Add(minimalQCow2[:])
	// The same bytes seed both the body and header generator. A zero-filled
	// input lets header generation finish so the seed run reaches the handler.
	f.Add(make([]byte, 1024))

	url := uploaderURL + uploadPath
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzz.ProcessRequests(t, data, url, http.MethodPost, http.MethodPut)
	})
}

func startUploaderServer(tb testing.TB, mockURL string) string {
	tb.Helper()

	tb.Setenv(common.UploaderDestinationEndpoint, strings.TrimPrefix(mockURL, "http://")+"/uploader")
	tb.Setenv(common.UploaderDestinationAuthConfig, "testdata/auth.json")

	uploaderServer, err := NewUploadServer(addr, 0, "", "", "", "", cryptowatch.CryptoConfig{})
	if err != nil {
		tb.Fatalf("failed to initialize uploader server; %v", err)
	}

	srv := uploaderServer.(*uploadServerApp)
	srv.keepAlive = true
	srv.keepConcurrent = true
	srv.destInsecure = true

	// Run exits when an upload is rejected. Exercise the same HTTP handler in a
	// test server so malformed inputs do not stop it for all subsequent inputs.
	// Drain its lifecycle notifications without calling testing.F from a goroutine.
	stopErrors := make(chan struct{})
	errorsStopped := make(chan struct{})
	go func() {
		defer close(errorsStopped)
		for {
			select {
			case <-srv.errChan:
			case <-stopErrors:
				return
			}
		}
	}()
	tb.Cleanup(func() {
		close(stopErrors)
		<-errorsStopped
	})

	server := httptest.NewServer(srv)
	tb.Cleanup(server.Close)

	return server.URL
}

func TestFuzzUploaderServerSurvivesRejectedUploads(t *testing.T) {
	mockURL := startDVCRMockServer(t)
	uploaderURL := startUploaderServer(t, mockURL)
	client := &http.Client{Timeout: 5 * time.Second}

	// More than one failed request must complete: lifecycle errors must not
	// block the handler or stop the shared fuzz server.
	for range 3 {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			for _, body := range [][]byte{nil, []byte("invalid snappy stream")} {
				req, err := http.NewRequest(method, uploaderURL+uploadPath, bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set(common.UploadContentTypeHeader, common.BlockdeviceClone)
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("rejected upload request: %v", err)
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusInternalServerError {
					t.Fatalf("rejected upload status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
				}
			}
		}
	}

	resp, err := client.Get(uploaderURL + uploadPath)
	if err != nil {
		t.Fatalf("uploader stopped after rejected uploads: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("uploader readiness status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func startDVCRMockServer(tb testing.TB) string {
	tb.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /v2/uploader/blobs/uploads/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Location", "/v2/uploader/blobs/uploads/test_data")
		w.WriteHeader(http.StatusAccepted)
	})

	mux.HandleFunc("PATCH /v2/uploader/blobs/uploads/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Add("Location", fmt.Sprintf("/v2/uploader/blobs/uploads/%s", id))
		w.WriteHeader(http.StatusAccepted)
	})

	mux.HandleFunc("PUT /v2/uploader/blobs/uploads/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Add("Location", fmt.Sprintf("/v2/uploader/blobs/uploads/%s", id))
		w.WriteHeader(http.StatusCreated)
	})

	mux.HandleFunc("GET /v2/uploader/blobs/uploads/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Add("Location", fmt.Sprintf("/v2/uploader/blobs/uploads/%s", id))
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("HEAD /v2/uploader/manifests/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/octet-stream")
		// 10 is random value
		w.Header().Add("Content-Length", "10")
		// random digest
		w.Header().Add("Docker-Content-Digest", "sha256:af3ca10a606165f3cad5226c504cea77b9f5169df6a536b26aeffd2e651c0ada")
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("PUT /v2/uploader/manifests/latest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /v2/uploader/manifests/latest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /v2/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	tb.Cleanup(server.Close)

	return server.URL
}
