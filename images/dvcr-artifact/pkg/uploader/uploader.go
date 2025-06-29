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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/containerized-data-importer/pkg/common"
	"kubevirt.io/containerized-data-importer/pkg/importer"
	"kubevirt.io/containerized-data-importer/pkg/util"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/auth"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/monitoring"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/registry"
	dtls "github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/tls"
)

const (
	healthzPort = 8080
	healthzPath = "/healthz"
	uploadPath  = "/upload"
)

// UploadServer is the interface to uploadServerApp
type UploadServer interface {
	Run() error
	PreallocationApplied() bool
}

type uploadServerApp struct {
	bindAddress    string
	bindPort       int
	tlsKey         string
	tlsCert        string
	clientCert     string
	clientName     string
	keyFile        string
	certFile       string
	mux            *http.ServeMux
	uploading      bool
	doneChan       chan struct{}
	errChan        chan error
	keepAlive      bool
	keepConcurrent bool
	mutex          sync.Mutex

	healthzServer *http.Server
	uploadServer  *http.Server

	destImageName string
	destUsername  string
	destPassword  string
	destInsecure  bool
}

type imageReadCloser func(*http.Request) (io.ReadCloser, error)

func bodyReadCloser(r *http.Request) (io.ReadCloser, error) {
	return r.Body, nil
}

// NewUploadServer returns a new instance of uploadServerApp
func NewUploadServer(bindAddress string, bindPort int, tlsKey, tlsCert, clientCert, clientName string) (UploadServer, error) {
	server := &uploadServerApp{
		bindAddress: bindAddress,
		bindPort:    bindPort,
		tlsKey:      tlsKey,
		tlsCert:     tlsCert,
		clientCert:  clientCert,
		clientName:  clientName,
		mux:         http.NewServeMux(),
		doneChan:    make(chan struct{}),
		errChan:     make(chan error),
	}

	err := server.parseOptions()
	if err != nil {
		return nil, err
	}

	server.mux.HandleFunc(uploadPath, server.uploadHandler(bodyReadCloser))

	return server, nil
}

func (app *uploadServerApp) parseOptions() error {
	app.destImageName, _ = util.ParseEnvVar(common.UploaderDestinationEndpoint, false)
	app.destInsecure, _ = strconv.ParseBool(os.Getenv(common.DestinationInsecureTLSVar))

	app.destUsername, _ = util.ParseEnvVar(common.UploaderDestinationAccessKeyID, false)
	app.destPassword, _ = util.ParseEnvVar(common.UploaderDestinationSecretKey, false)
	if app.destUsername == "" && app.destPassword == "" {
		destAuthConfig, _ := util.ParseEnvVar(common.UploaderDestinationAuthConfig, false)
		if destAuthConfig != "" {
			authFile, err := auth.RegistryAuthFile(destAuthConfig)
			if err != nil {
				return fmt.Errorf("error parsing destination auth config: %w", err)
			}

			app.destUsername, app.destPassword, err = auth.CredsFromRegistryAuthFile(authFile, app.destImageName)
			if err != nil {
				return fmt.Errorf("error getting creds from destination auth config: %w", err)
			}
		}
	}

	return nil
}

func (app *uploadServerApp) Run() error {
	uploadListener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", app.bindAddress, app.bindPort))
	if err != nil {
		return errors.Wrap(err, "Error creating upload listerner")
	}

	healthzListener, err := net.Listen("tcp", fmt.Sprintf(":%d", healthzPort))
	if err != nil {
		return errors.Wrap(err, "Error creating healthz listerner")
	}

	app.uploadServer, err = app.createUploadServer()
	if err != nil {
		return errors.Wrap(err, "Error creating upload http server")
	}

	app.healthzServer = app.createHealthzServer()

	go func() {
		// maybe bind port was 0 (unit tests) assign port here
		app.bindPort = uploadListener.Addr().(*net.TCPAddr).Port

		if app.keyFile != "" && app.certFile != "" {
			app.errChan <- app.uploadServer.ServeTLS(uploadListener, app.certFile, app.keyFile)
			return
		}

		// not sure we want to support this code path
		app.errChan <- app.uploadServer.Serve(uploadListener)
	}()

	go func() {
		app.errChan <- app.healthzServer.Serve(healthzListener)
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
	case err = <-app.errChan:
	case <-app.doneChan:
		klog.Info("Shutting down http server after successful upload")
	case <-exit:
		klog.Errorf("Shutting down http server")
	}

	app.shutdown()

	return err
}

func (app *uploadServerApp) shutdown() {
	if err := app.healthzServer.Shutdown(context.Background()); err != nil {
		klog.Errorf("failed to shutdown healthzServer; %v", err)
	}
	if err := app.uploadServer.Shutdown(context.Background()); err != nil {
		klog.Errorf("failed to shutdown uploadServer; %v", err)
	}
}

func (app *uploadServerApp) createUploadServer() (*http.Server, error) {
	server := &http.Server{
		Handler: app,
	}

	if app.tlsKey != "" && app.tlsCert != "" {
		certDir, err := os.MkdirTemp("", "uploadserver-tls")
		if err != nil {
			return nil, errors.Wrap(err, "Error creating cert dir")
		}

		app.keyFile = filepath.Join(certDir, "tls.key")
		app.certFile = filepath.Join(certDir, "tls.crt")

		err = os.WriteFile(app.keyFile, []byte(app.tlsKey), 0o600)
		if err != nil {
			return nil, errors.Wrap(err, "Error creating key file")
		}

		err = os.WriteFile(app.certFile, []byte(app.tlsCert), 0o600)
		if err != nil {
			return nil, errors.Wrap(err, "Error creating cert file")
		}
	}

	if app.clientCert != "" {
		certPool, err := dtls.NewCertPool(app.clientCert)
		if err != nil {
			return nil, err
		}
		server.TLSConfig = dtls.NewBuilder().WithClientCAs(certPool).Build()
	}

	return server, nil
}

func (app *uploadServerApp) createHealthzServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc(healthzPath, app.healthzHandler)
	return &http.Server{Handler: mux}
}

func (app *uploadServerApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app.mux.ServeHTTP(w, r)
}

func (app *uploadServerApp) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	if _, err := io.WriteString(w, "OK"); err != nil {
		klog.Errorf("healthzHandler: failed to send response; %v", err)
	}
}

func (app *uploadServerApp) validateShouldHandleRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.WriteHeader(http.StatusNotFound)
		return false
	}
	if r.TLS != nil {
		found := false

		for _, cert := range r.TLS.PeerCertificates {
			if cert.Subject.CommonName == app.clientName {
				found = true
				break
			}
		}

		if !found {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
	} else {
		klog.V(3).Infof("Handling HTTP connection")
	}

	app.mutex.Lock()
	defer app.mutex.Unlock()

	if app.uploading && !app.keepConcurrent {
		klog.Warning("Got concurrent upload request")
		w.WriteHeader(http.StatusServiceUnavailable)
		return false
	}

	app.uploading = true

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

func (app *uploadServerApp) processUpload(irc imageReadCloser, w http.ResponseWriter, r *http.Request, dvContentType cdiv1.DataVolumeContentType) {
	if !app.validateShouldHandleRequest(w, r) {
		return
	}

	cdiContentType := r.Header.Get(common.UploadContentTypeHeader)

	klog.Infof("Content type header is %q\n", cdiContentType)

	readCloser, err := irc(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}

	err = app.upload(readCloser, cdiContentType, dvContentType, parseHTTPHeader(r))

	app.mutex.Lock()
	defer app.mutex.Unlock()

	if err != nil {
		klog.Errorf("Saving stream failed: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		app.errChan <- err

		return
	}

	if !app.keepAlive {
		close(app.doneChan)
	}
}

func (app *uploadServerApp) uploadHandler(irc imageReadCloser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app.processUpload(irc, w, r, cdiv1.DataVolumeKubeVirt)
	}
}

func (app *uploadServerApp) PreallocationApplied() bool {
	panic("not implemented")
}

func (app *uploadServerApp) upload(stream io.ReadCloser, sourceContentType string, dvContentType cdiv1.DataVolumeContentType, contentLength int) error {
	durCollector := monitoring.NewDurationCollector()

	uds := importer.NewUploadDataSource(newContentReader(stream, sourceContentType), dvContentType, contentLength)
	defer uds.Close()

	processor, err := registry.NewDataProcessor(uds, registry.DestinationRegistry{
		ImageName: app.destImageName,
		Username:  app.destUsername,
		Password:  app.destPassword,
		Insecure:  app.destInsecure,
	}, "", "")
	if err != nil {
		return err
	}

	res, err := processor.Process(context.Background())
	if err != nil {
		return monitoring.WriteImportFailureMessage(err)
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
