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

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	log "log/slog"
	"net"
	"net/http"
	"sync"

	logutil "github.com/deckhouse/kube-api-rewriter/pkg/log"
	"github.com/deckhouse/kube-api-rewriter/pkg/tls/certmanager"
)

// HTTPServer starts HTTP server with root handler using listen address.
// Implements Runnable interface to be able to stop server.
type HTTPServer struct {
	InstanceDesc string
	ListenAddr   string
	RootHandler  http.Handler
	CertManager  certmanager.CertificateManager
	Err          error

	initLock sync.Mutex
	stopped  bool

	listener net.Listener
	instance *http.Server
}

// init checks if listen is possible and creates new HTTP server instance.
// initLock is used to avoid data races with the Stop method.
func (s *HTTPServer) init() bool {
	s.initLock.Lock()
	defer s.initLock.Unlock()
	if s.stopped {
		// Stop was called earlier.
		return false
	}

	l, err := net.Listen("tcp", s.ListenAddr)
	if err != nil {
		s.Err = err
		log.Error(fmt.Sprintf("%s: listen on %s err: %s", s.InstanceDesc, s.ListenAddr, err))
		return false
	}
	s.listener = l
	log.Info(fmt.Sprintf("%s: listen for incoming requests on %s", s.InstanceDesc, s.ListenAddr))

	mux := http.NewServeMux()
	mux.Handle("/", s.RootHandler)

	s.instance = &http.Server{
		Handler: mux,
	}
	return true
}

func (s *HTTPServer) Start() {
	if !s.init() {
		return
	}

	// Start serving HTTP requests, block until server instance stops or returns an error.
	var err error
	if s.CertManager != nil {
		go s.CertManager.Start()
		s.setupTLS()
		err = s.instance.ServeTLS(s.listener, "", "")
	} else {
		err = s.instance.Serve(s.listener)
	}
	// Ignore closed error: it's a consequence of stop.
	if err != nil {
		switch {
		case errors.Is(err, http.ErrServerClosed):
		case errors.Is(err, net.ErrClosed):
		default:
			s.Err = err
		}
	}
	return
}

func (s *HTTPServer) setupTLS() {
	s.instance.TLSConfig = &tls.Config{
		GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert := s.CertManager.Current()
			if cert == nil {
				return nil, errors.New("no server certificate, server is not yet ready to receive traffic")
			}
			return cert, nil
		},
	}
}

// Stop shutdowns HTTP server instance and close a done channel.
// Stop and init may be run in parallel, so initLock is used to wait until
// variables are initialized.
func (s *HTTPServer) Stop() {
	s.initLock.Lock()
	defer s.initLock.Unlock()

	if s.stopped {
		return
	}
	s.stopped = true

	if s.CertManager != nil {
		s.CertManager.Stop()
	}
	// Shutdown instance if it was initialized.
	if s.instance != nil {
		log.Info(fmt.Sprintf("%s: stop", s.InstanceDesc))
		err := s.instance.Shutdown(context.Background())
		// Ignore ErrClosed.
		if err != nil {
			switch {
			case errors.Is(err, http.ErrServerClosed):
			case errors.Is(err, net.ErrClosed):
			case s.Err != nil:
				// log error to not reset runtime error.
				log.Error(fmt.Sprintf("%s: stop instance", s.InstanceDesc), logutil.SlogErr(err))
			default:
				s.Err = err
			}
		}
	}
}

// ConstructListenAddr return ip:port with defaults.
func ConstructListenAddr(addr, port, defaultAddr, defaultPort string) string {
	if addr == "" {
		addr = defaultAddr
	}
	if port == "" {
		port = defaultPort
	}
	return addr + ":" + port
}
