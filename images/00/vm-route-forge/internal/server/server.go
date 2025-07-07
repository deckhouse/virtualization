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
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"

	"vm-route-forge/internal/runnablegroup"
)

const (
	defaultGracefulShutdownPeriod = 30 * time.Second
	defaultReadinessEndpoint      = "/readyz"
	defaultLivenessEndpoint       = "/healthz"
)

type Server struct {
	runnableGroup           *runnablegroup.RunnableGroup
	gracefulShutdownTimeout time.Duration
	healthProbeListener     net.Listener
	pprofListener           net.Listener
	readyzHandler           http.Handler
	healthzHandler          http.Handler
	readinessEndpointRoute  string
	livenessEndpointRoute   string

	client kubernetes.Interface
	log    logr.Logger
}

func (s *Server) Run(ctx context.Context) error {
	if s.healthProbeListener != nil {
		s.addHealthProbeServer()
	}
	if s.pprofListener != nil {
		s.addPprofServer()
	}
	return s.runnableGroup.Run(ctx)
}

func (s *Server) addHealthProbeServer() {
	mux := http.NewServeMux()
	srv := NewHTTPServer(mux)

	mux.Handle(s.readinessEndpointRoute, http.StripPrefix(s.readinessEndpointRoute, s.getReadyzHandler()))
	// Append '/' suffix to handle subpaths
	mux.Handle(s.readinessEndpointRoute+"/", http.StripPrefix(s.readinessEndpointRoute, s.getReadyzHandler()))

	mux.Handle(s.livenessEndpointRoute, http.StripPrefix(s.livenessEndpointRoute, s.getHealthzHandler()))
	// Append '/' suffix to handle subpaths
	mux.Handle(s.livenessEndpointRoute+"/", http.StripPrefix(s.livenessEndpointRoute, s.getHealthzHandler()))

	s.Add(&httpServer{
		name:                    "health",
		gracefulShutdownTimeout: s.gracefulShutdownTimeout,
		listener:                s.healthProbeListener,
		server:                  srv,
		log:                     s.log,
	})
}

func (s *Server) addPprofServer() {
	mux := http.NewServeMux()
	srv := NewHTTPServer(mux)

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	s.Add(&httpServer{
		name:                    "pprof",
		gracefulShutdownTimeout: s.gracefulShutdownTimeout,
		server:                  srv,
		log:                     s.log,
		listener:                s.pprofListener,
	})
}

func (s *Server) Add(r runnablegroup.Runnable) {
	s.runnableGroup.Add(r)
}

type Options struct {
	HealthProbeBindAddress  string
	PprofBindAddress        string
	ReadinessEndpointRoute  string
	LivenessEndpointRoute   string
	GracefulShutdownTimeout *time.Duration
	ReadyzHandler           http.Handler
	HealthzHandler          http.Handler
}

func setOptionsDefault(options Options) Options {
	if options.GracefulShutdownTimeout == nil {
		gracefulShutdownTimeout := defaultGracefulShutdownPeriod
		options.GracefulShutdownTimeout = &gracefulShutdownTimeout
	}
	if options.ReadinessEndpointRoute == "" {
		options.ReadinessEndpointRoute = defaultReadinessEndpoint
	}
	if options.LivenessEndpointRoute == "" {
		options.LivenessEndpointRoute = defaultLivenessEndpoint
	}
	return options
}

func NewServer(client kubernetes.Interface, options Options, log logr.Logger) (*Server, error) {
	options = setOptionsDefault(options)

	// Create health probes listener. This will throw an error if the bind
	// address is invalid or already in use.
	healthProbeListener, err := defaultListener(options.HealthProbeBindAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create new healthprobe listener: %w", err)
	}
	pprofListener, err := defaultListener(options.PprofBindAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create new pprof listener: %w", err)
	}

	return &Server{
		healthProbeListener:     healthProbeListener,
		pprofListener:           pprofListener,
		gracefulShutdownTimeout: *options.GracefulShutdownTimeout,
		readinessEndpointRoute:  options.ReadinessEndpointRoute,
		livenessEndpointRoute:   options.LivenessEndpointRoute,
		runnableGroup:           runnablegroup.NewRunnableGroup(),
		readyzHandler:           options.ReadyzHandler,
		healthzHandler:          options.HealthzHandler,
		client:                  client,
		log:                     log,
	}, nil
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
