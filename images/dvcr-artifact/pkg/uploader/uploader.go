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
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/containerized-data-importer/pkg/common"
	"kubevirt.io/containerized-data-importer/pkg/importer"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/monitoring"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/registry"
)

const (
	defaultHealthzPort = 8080
	healthzPath        = "/healthz"
	uploadPath         = "/upload"
)

// Destination describes the DVCR registry the uploaded image is pushed to.
type Destination struct {
	Endpoint string
	Username string
	Password string
	// Insecure skips TLS verification of the DVCR registry certificate.
	Insecure bool
	// CABundle is a path to a PEM file or a directory with PEM files used to
	// verify the DVCR registry certificate.
	CABundle string
}

// Server receives an uploaded image over HTTP(S) and pushes it to the DVCR
// registry. It is created either from Options.Complete or directly via NewServer
// with an already-built *tls.Config.
type Server struct {
	address     string
	healthzPort int
	// tlsConfig is nil for plain HTTP; when set the server serves HTTPS.
	tlsConfig   *tls.Config
	destination Destination
	// checksums the uploaded data has to match, keyed by algorithm. Empty when
	// the resource asks for no verification.
	checksums map[string]string

	mux            *http.ServeMux
	uploading      bool
	keepAlive      bool
	keepConcurrent bool
	mutex          sync.Mutex

	startListeningChan chan struct{}
	stopListeningChan  chan struct{}
	errChan            chan error

	healthzServer *http.Server
	uploadServer  *http.Server

	// boundPort is the actual upload port after Listen (useful when the
	// requested port was 0, e.g. in tests).
	boundPort int
}

// NewServer builds an upload server from a minimal set of already-prepared
// parameters. TLS is fully described by tlsConfig (nil means plain HTTP), so the
// server does not deal with certificate files or crypto options itself.
func NewServer(address string, healthzPort int, tlsConfig *tls.Config, destination Destination, checksums map[string]string) *Server {
	if healthzPort == 0 {
		healthzPort = defaultHealthzPort
	}

	s := &Server{
		address:     address,
		healthzPort: healthzPort,
		tlsConfig:   tlsConfig,
		destination: destination,
		checksums:   checksums,

		mux:                http.NewServeMux(),
		stopListeningChan:  make(chan struct{}),
		errChan:            make(chan error),
		startListeningChan: make(chan struct{}),
	}

	s.mux.HandleFunc(uploadPath, s.uploadHandler())

	return s
}

func (s *Server) Run() error {
	uploadListener, err := net.Listen("tcp", s.address)
	if err != nil {
		return errors.Wrap(err, "Error creating upload listener")
	}
	s.boundPort = uploadListener.Addr().(*net.TCPAddr).Port

	healthzListener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.healthzPort))
	if err != nil {
		return errors.Wrap(err, "Error creating healthz listener")
	}

	close(s.startListeningChan)

	s.uploadServer = &http.Server{
		Handler:   s.mux,
		TLSConfig: s.tlsConfig,
	}
	s.healthzServer = s.createHealthzServer()

	go func() {
		if s.tlsConfig != nil {
			// Certificates are already loaded into the server TLSConfig.
			s.errChan <- s.uploadServer.ServeTLS(uploadListener, "", "")
			return
		}

		s.errChan <- s.uploadServer.Serve(uploadListener)
	}()

	go func() {
		s.errChan <- s.healthzServer.Serve(healthzListener)
	}()

	promCertsDir, err := os.MkdirTemp("", "certsdir")
	if err != nil {
		return fmt.Errorf("error creating prometheus certs directory: %w", err)
	}
	defer os.RemoveAll(promCertsDir)
	prometheusutil.StartPrometheusEndpoint(promCertsDir)

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM)

	select {
	case err = <-s.errChan:
	case <-s.stopListeningChan:
		klog.Info("Shutting down http server after successful upload")
	case <-exit:
		klog.Errorf("Shutting down http server")
	}

	s.shutdown()

	return err
}

func (s *Server) shutdown() {
	if err := s.healthzServer.Shutdown(context.Background()); err != nil {
		klog.Errorf("failed to shutdown healthzServer; %v", err)
	}
	if err := s.uploadServer.Shutdown(context.Background()); err != nil {
		klog.Errorf("failed to shutdown uploadServer; %v", err)
	}
}

func (s *Server) createHealthzServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(healthzPath, s.healthzHandler)
	return &http.Server{Handler: mux}
}

func (s *Server) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	if _, err := io.WriteString(w, "OK"); err != nil {
		klog.Errorf("healthzHandler: failed to send response; %v", err)
	}
}

// validateShouldHandleRequest handles the readiness signal (GET), rejects
// unsupported methods and guards against concurrent uploads. Client certificate
// verification (mTLS) is enforced by the TLS layer via the server's tls.Config,
// so it is intentionally not repeated here.
func (s *Server) validateShouldHandleRequest(w http.ResponseWriter, r *http.Request) bool {
	// This method is used to signal that ingress is configured and the server can upload user data.
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return false
	}

	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.WriteHeader(http.StatusNotFound)
		return false
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.uploading && !s.keepConcurrent {
		klog.Warning("Got concurrent upload request")
		w.WriteHeader(http.StatusServiceUnavailable)
		return false
	}

	s.uploading = true

	return true
}

func parseHTTPHeader(resp *http.Request) int {
	val, ok := resp.Header["Content-Length"]
	if ok {
		total, err := strconv.ParseUint(val[0], 10, 64)
		if err != nil {
			klog.Errorf("could not convert content length, got %v", err)
		}
		klog.V(3).Infof("Content length: %d\n", total)

		return int(total)
	}

	return 0
}

func (s *Server) processUpload(w http.ResponseWriter, r *http.Request, dvContentType cdiv1.DataVolumeContentType) {
	if !s.validateShouldHandleRequest(w, r) {
		return
	}

	cdiContentType := r.Header.Get(common.UploadContentTypeHeader)

	klog.Infof("Content type header is %q\n", cdiContentType)

	s.mutex.Lock()
	defer s.mutex.Unlock()

	err := s.upload(r.Body, cdiContentType, dvContentType, parseHTTPHeader(r))
	if err != nil {
		klog.Errorf("Saving stream failed: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		s.errChan <- err

		return
	}

	if !s.keepAlive {
		close(s.stopListeningChan)
	}
}

func (s *Server) uploadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.processUpload(w, r, cdiv1.DataVolumeKubeVirt)
	}
}

func (s *Server) upload(stream io.ReadCloser, sourceContentType string, dvContentType cdiv1.DataVolumeContentType, contentLength int) error {
	durCollector := monitoring.NewDurationCollector()

	uds := importer.NewUploadDataSource(newContentReader(stream, sourceContentType), dvContentType, contentLength)
	defer uds.Close()

	processor, err := registry.NewDataProcessor(uds, registry.DestinationRegistry{
		ImageName: s.destination.Endpoint,
		Username:  s.destination.Username,
		Password:  s.destination.Password,
		Insecure:  s.destination.Insecure,
		CABundle:  s.destination.CABundle,
	}, s.checksums)
	if err != nil {
		return err
	}

	res, err := processor.Process(context.Background())
	if err != nil {
		// The controller learns about the failure from the termination message,
		// but the client has to learn it from the response: answering 200 would
		// hide a rejected upload - a checksum mismatch, most notably - behind an
		// apparent success.
		if writeErr := monitoring.WriteImportFailureMessage(err); writeErr != nil {
			klog.Errorf("Failed to write the termination message: %s", writeErr)
		}

		return err
	}

	return monitoring.WriteImportCompleteMessage(res.SourceImageSize, res.VirtualSize, res.AvgSpeed, res.Format, durCollector.Collect())
}

func newContentReader(stream io.ReadCloser, contentType string) io.ReadCloser {
	if contentType == common.BlockdeviceClone {
		return newSnappyReadCloser(stream)
	}

	return stream
}

func newSnappyReadCloser(stream io.ReadCloser) io.ReadCloser {
	return io.NopCloser(snappy.NewReader(stream))
}
