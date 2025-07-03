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
	"net/http"
	"os"
	"testing"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/fuzz"
	"kubevirt.io/containerized-data-importer/pkg/common"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"
)

const (
	UPLOADER_FUZZ_PORT = "UPLOADER_FUZZ_PORT"
	UPLOADER_MOCK_PORT = "UPLOADER_MOCK_PORT"
)

func FuzzUploader(f *testing.F) {
	uploaderPort, err := fuzz.GetFreePort()
	if err != nil {
		f.Fatalf("failed to parse uploaderEnv: %v", err)
	}

	mockPort, err := fuzz.GetFreePort()
	if err != nil {
		f.Fatalf("failed to parse mockEnv: %v", err)
	}

	addr := "127.0.0.1"
	url := fmt.Sprintf("http://%s:%d/upload", addr, uploaderPort)

	startUploaderServer(f, addr, uploaderPort, mockPort)

	startDVCRMockServer(f, addr, mockPort)

	// 512 bytes is the minimum size of a qcow2 image
	minimalQCow2 := [512]byte{
		0x51, 0x46, 0x49, 0xfb, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00,
	}
	f.Add(minimalQCow2[:])

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzz.ProcessRequests(t, data, url, http.MethodPut, http.MethodPost)
	})
}

func startUploaderServer(tb testing.TB, addr string, uploaderPort, mockPort int) *uploadServerApp {
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

	go func() {
		if err := uploaderServer.Run(); err != nil {
			tb.Fatalf("failed to run uploader server: %v", err)
		}
	}()

	return srv
}

func startDVCRMockServer(tb testing.TB, addr string, port int) {
	tb.Helper()

	url := fmt.Sprintf("%s:%d", addr, port)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /v2/uploader/blobs/uploads/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Location", fmt.Sprintf("/v2/uploader/blobs/uploads/test_data"))
		w.WriteHeader(http.StatusAccepted)
	})

	mux.HandleFunc("GET /v2/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		if err := http.ListenAndServe(url, mux); err != nil {
			tb.Fatalf("failed to listen and serve mock server: %v", err)
		}
	}()
}
