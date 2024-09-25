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
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	logutil "kube-api-proxy/pkg/log"
)

func NewHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		MaxHeaderBytes:    1 << 20,
		IdleTimeout:       90 * time.Second,
		ReadHeaderTimeout: 32 * time.Second,
	}
}

type httpServer struct {
	mu                      sync.Mutex
	name                    string
	gracefulShutdownTimeout time.Duration
	server                  *http.Server
	listener                net.Listener
	log                     *slog.Logger
	ctx                     context.Context
	cancel                  context.CancelFunc
}

func (s *httpServer) Run(ctx context.Context) error {
	log := s.log.With("httpServerName", s.name, "addr", s.listener.Addr())

	serverShutdown := make(chan struct{})
	go func() {
		<-ctx.Done()
		log.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.gracefulShutdownTimeout)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			log.Error("error shutting down server", logutil.SlogErr(err))
		}
		close(serverShutdown)
	}()

	log.Info("starting server")
	if err := s.server.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-serverShutdown
	return nil
}

func (s *httpServer) Start() {
	s.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel
	s.mu.Unlock()

	if err := s.Run(ctx); err != nil {
		s.log.Error("error starting server", slog.String("httpServerName", s.name), logutil.SlogErr(err))
	}
}

func (s *httpServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.ctx.Done():
	default:
		s.cancel()
	}
}
