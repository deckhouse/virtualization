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

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type Server interface {
	Run(ctx context.Context, opts ...Option) error
}

type endpointer interface {
	Handler() http.Handler
	Path() string
}

func NewServer(addr string, regs ...endpointer) (Server, error) {
	mux := http.NewServeMux()
	for _, endpoint := range regs {
		mux.Handle(endpoint.Path(), endpoint.Handler())
	}

	listener, err := defaultListener(addr)
	if err != nil {
		return nil, err
	}

	srv := httpServer{
		gracefulShutdownTimeout: 10 * time.Second,
		listener:                listener,
		server: &http.Server{
			Handler:           mux,
			MaxHeaderBytes:    1 << 20,
			IdleTimeout:       90 * time.Second,
			ReadHeaderTimeout: 32 * time.Second,
		},
	}

	return &srv, nil
}

type httpServer struct {
	gracefulShutdownTimeout time.Duration
	server                  *http.Server
	listener                net.Listener
}

func (s *httpServer) Run(ctx context.Context, opts ...Option) error {
	serverShutdown := make(chan struct{})
	go func() {
		<-ctx.Done()
		log.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.gracefulShutdownTimeout)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			log.Error("error shutting down server", log.Err(err))
		}
		close(serverShutdown)
	}()

	log.Info("starting server")
	o := options{}
	for _, opt := range opts {
		opt.Apply(&o)
	}

	var err error
	if o.TLS != nil {
		err = s.server.ServeTLS(s.listener, o.TLS.CertFile, o.TLS.KeyFile)
	} else {
		err = s.server.Serve(s.listener)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-serverShutdown
	return nil
}

func defaultListener(addr string) (net.Listener, error) {
	if addr == "" || addr == "0" {
		return nil, nil
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("error listening on %s: %w", addr, err)
	}
	return ln, nil
}
