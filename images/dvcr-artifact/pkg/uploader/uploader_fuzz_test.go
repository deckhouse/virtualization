package uploader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/fuzz"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"
)

func FuzzUploader(f *testing.F) {
	addr := "127.0.0.1"
	port := 8000
	uri := fmt.Sprintf("http://%s:%d/upload", addr, port)

	initializeUploaderServer(f, addr, port)

	dvcrServer := createDVCRMockServer()
	defer dvcrServer.Close()

	minimalQCow2 := [512]byte{
		0x51, 0x46, 0x49, 0xfb, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00,
	}
	f.Add(minimalQCow2[:])

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzz.ProcessRequests(t, data, uri, http.MethodPut, http.MethodPost)
	})
}

func initializeUploaderServer(tb testing.TB, addr string, port int) *uploadServerApp {
	tb.Helper()

	uploaderServer, err := NewUploadServer(addr, port, "", "", "", "", cryptowatch.CryptoConfig{})
	if err != nil {
		tb.Fatalf("failed to initialize uploader server; %v", err)
	}

	go func() {
		if err := uploaderServer.Run(); err != nil {
			tb.Fatalf("failed to start uploader server; %v", err)
		}
	}()

	srv := uploaderServer.(*uploadServerApp)
	srv.keepAlive = true
	srv.keepCuncurrent = true

	return srv
}

var jwksMock = `{ "k": "v" }`

func createDVCRMockServer() *httptest.Server {
	mux := http.NewServeMux()

	server := httptest.NewServer(mux)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// json.NewEncoder(w).Encode(config)
	})

	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		// http.Redirect(w, r, "http://localhost:8250/oidc/callback?code=mock-code", http.StatusFound)
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(jwksMock)
		if err != nil {
			log.Error("failed to encode jwks", err)
		}
	})

	return server
}
