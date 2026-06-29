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
		addr:    addr,
		handler: handler,
	}, nil
}

type tcpServer struct {
	addr    string
	handler func([]byte) error
}

func (s *tcpServer) Run(ctx context.Context, opts ...Option) error {
	o := options{}
	for _, opt := range opts {
		opt.Apply(&o)
	}

	var listener net.Listener
	var err error
	if o.TLS != nil {
		// Fail fast if the certificate material is missing or invalid at startup.
		if _, err = loadServerTLSConfig(o.TLS); err != nil {
			return err
		}

		// ponytail: reload cert and client CA from disk on every handshake so
		// secret rotation needs no pod restart. Handshakes are rare (vector keeps
		// a persistent connection), so per-handshake disk reads are negligible.
		cfg := &tls.Config{
			GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
				return loadServerTLSConfig(o.TLS)
			},
		}
		listener, err = tls.Listen("tcp", s.addr, cfg)
	} else {
		listener, err = net.Listen("tcp", s.addr)
	}
	if err != nil {
		return err
	}

	defer listener.Close()

	// Accept connections in a loop that respects context cancellation
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	log.Debug("Server started", slog.String("address", s.addr))

	for {
		conn, err := listener.Accept()
		if err != nil {
			// Check if server is shutting down
			select {
			case <-ctx.Done():
				return nil
			default:
				log.Error("Error accepting connection", log.Err(err))
				continue
			}
		}

		// Handle each connection in its own goroutine
		go s.handleConnection(ctx, conn)
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
