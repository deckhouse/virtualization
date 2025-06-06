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

func FuzzUploader(f *testing.F) {
	addr := "127.0.0.1"
	url := fmt.Sprintf("http://%s:%d/upload", addr, 8000)

	startUploaderServer(f, addr, 8000)

	startDVCRMockServer(f, addr, 8400)

	minimalQCow2 := [512]byte{
		0x51, 0x46, 0x49, 0xfb, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00,
	}
	f.Add(minimalQCow2[:])

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzz.ProcessRequest(t, data, url, http.MethodPut)
	})
}

func startUploaderServer(tb testing.TB, addr string, port int) *uploadServerApp {
	tb.Helper()

	endpoint := fmt.Sprintf("%s:%d/uploader", addr, 8400)
	os.Setenv(common.UploaderDestinationEndpoint, endpoint)
	os.Setenv(common.UploaderDestinationAuthConfig, "testdata/auth.json")

	uploaderServer, err := NewUploadServer(addr, port, "", "", "", "", cryptowatch.CryptoConfig{})
	if err != nil {
		tb.Fatalf("failed to initialize uploader server; %v", err)
	}

	srv := uploaderServer.(*uploadServerApp)
	srv.keepAlive = true
	srv.keepCuncurrent = true
	srv.destInsecure = true

	go func() {
		if err := uploaderServer.Run(); err != nil {
			tb.Fatalf("failed to start uploader server; %v", err)
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
			tb.Fatalf("failed to start dvcr mock server; %v", err)
		}
	}()
}
