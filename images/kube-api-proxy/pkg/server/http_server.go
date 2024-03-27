package server

import (
	"context"
	"errors"
	"fmt"
	log "log/slog"
	"net"
	"net/http"
	"sync"

	logutil "kube-api-proxy/pkg/log"
)

// HTTPServer starts HTTP server with root handler using listen address.
// Implements Runnable interface to be able to stop server.
type HTTPServer struct {
	InstanceDesc string
	ListenAddr   string
	RootHandler  http.Handler
	CertFile     string
	KeyFile      string
	Err          error

	instance *http.Server
	stopOnce sync.Once
	stopped  bool
	doneCh   chan struct{}
}

func (s *HTTPServer) Start() {
	if s.stopped {
		return
	}
	if s.doneCh == nil {
		s.doneCh = make(chan struct{})
	}

	l, err := net.Listen("tcp", s.ListenAddr)
	if err != nil {
		s.Err = err
		return
	}
	log.Info(fmt.Sprintf("%s: listen for incoming requests on %s", s.InstanceDesc, s.ListenAddr))

	mux := http.NewServeMux()
	mux.Handle("/", s.RootHandler)

	s.instance = &http.Server{
		Handler: mux,
	}

	if s.CertFile != "" && s.KeyFile != "" {
		err = s.instance.ServeTLS(l, s.CertFile, s.KeyFile)
	} else {
		err = s.instance.Serve(l)
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

func (s *HTTPServer) Done() chan struct{} {
	return s.doneCh
}

func (s *HTTPServer) Stop() {
	s.stopOnce.Do(func() {
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
		close(s.doneCh)
	})
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
