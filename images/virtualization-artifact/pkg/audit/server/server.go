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
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/deckhouse/deckhouse/pkg/log"
)

// maxAuditEventSize bounds a single audit event line. The bufio.Scanner default
// is 64 KiB, which k8s audit events with large objects (managedFields, VM specs)
// can exceed — dropping the rest of the connection. 4 MiB leaves generous headroom.
const maxAuditEventSize = 4 * 1024 * 1024

type Server interface {
	Run(ctx context.Context, opts ...Option) error
}

func NewServer(addr string, handler func([]byte) error) (Server, error) {
	return &tcpServer{
		gracefulShutdownTimeout: 5 * time.Second,
		addr:                    addr,
		handler:                 handler,
	}, nil
}

type tcpServer struct {
	gracefulShutdownTimeout time.Duration
	addr                    string
	handler                 func([]byte) error
}

func (s *tcpServer) Run(ctx context.Context, opts ...Option) error {
	o := options{}
	for _, opt := range opts {
		opt.Apply(&o)
	}

	var listener net.Listener
	var err error
	if o.TLS != nil {
		// The pod is rolled on every certificate rotation (module policy), so the
		// cert is loaded once at startup; graceful shutdown drains connections so
		// that roll stays clean for the log-shipper.
		var cfg *tls.Config
		if cfg, err = loadServerTLSConfig(o.TLS); err != nil {
			return err
		}
		listener, err = tls.Listen("tcp", s.addr, cfg)
	} else {
		listener, err = net.Listen("tcp", s.addr)
	}
	if err != nil {
		return err
	}

	defer listener.Close()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		conns   = map[net.Conn]struct{}{}
		closing bool
	)

	// On shutdown: stop accepting and close every active connection. Closing a
	// tls.Conn sends close_notify, so the peer sees a clean EOF instead of a
	// truncated record ("bad record MAC") and can reconnect to another replica.
	go func() {
		<-ctx.Done()
		listener.Close()
		mu.Lock()
		closing = true
		for c := range conns {
			c.Close()
		}
		mu.Unlock()
	}()

	log.Debug("Server started", slog.String("address", s.addr))

	for {
		conn, err := listener.Accept()
		if err != nil {
			// Check if server is shutting down
			select {
			case <-ctx.Done():
				waitWithTimeout(&wg, s.gracefulShutdownTimeout)
				return nil
			default:
				log.Error("Error accepting connection", log.Err(err))
				continue
			}
		}

		mu.Lock()
		if closing {
			mu.Unlock()
			conn.Close()
			continue
		}
		conns[conn] = struct{}{}
		mu.Unlock()

		// Handle each connection in its own goroutine
		wg.Go(func() {
			defer func() {
				mu.Lock()
				delete(conns, conn)
				mu.Unlock()
			}()
			s.handleConnection(ctx, conn)
		})
	}
}

// waitWithTimeout blocks until the WaitGroup drains or the timeout elapses,
// so in-flight connections finish cleanly without holding shutdown forever.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// loadServerTLSConfig reads the server certificate and the client CA from disk
// and builds a config that requires and verifies a client certificate (mTLS),
// so only clients holding a certificate signed by our CA can submit audit events.
func loadServerTLSConfig(t *TLSPair) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS key pair: %w", err)
	}

	caPEM, err := os.ReadFile(t.CaFile)
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}

	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no valid certificate found in CA file %q", t.CaFile)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
	}, nil
}

func (s *tcpServer) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	log.Debug("New connection", slog.String("remote", conn.RemoteAddr().String()))

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxAuditEventSize)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}

		if err := s.handler(scanner.Bytes()); err != nil {
			log.Warn("Failed to process audit event", log.Err(err))
		}
	}

	if err := scanner.Err(); err != nil {
		// A peer dropping the connection mid-stream (pod roll, vector reconnect)
		// surfaces here as EOF / reset / "bad record MAC". It is expected churn,
		// not a server fault, so it stays at debug level.
		log.Debug("Connection closed with error", slog.String("remote", conn.RemoteAddr().String()), log.Err(err))
	}
}
