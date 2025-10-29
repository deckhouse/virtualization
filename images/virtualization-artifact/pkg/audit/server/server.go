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
	"time"

	"github.com/deckhouse/deckhouse/pkg/log"
)

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
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return fmt.Errorf("fail to get system cert pool: %w", err)
		}

		if caCertPEM, err := os.ReadFile(o.TLS.CaFile); err != nil {
			return fmt.Errorf("fail to read CA PEM: %w", err)
		} else if ok := certPool.AppendCertsFromPEM(caCertPEM); !ok {
			return fmt.Errorf("invalid cert in CA PEM: %w", err)
		}

		cert, err := tls.LoadX509KeyPair(o.TLS.CertFile, o.TLS.KeyFile)
		if err != nil {
			return err
		}

		config := &tls.Config{
			RootCAs:      certPool,
			Certificates: []tls.Certificate{cert},
		}

		listener, err = tls.Listen("tcp", s.addr, config)
		if err != nil {
			return err
		}
	} else {
		listener, err = net.Listen("tcp", s.addr)
		if err != nil {
			return err
		}
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

func (s *tcpServer) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	log.Debug("New connection", slog.String("remote", conn.RemoteAddr().String()))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
			line := scanner.Bytes()

			err := s.handler(line)
			if err != nil {
				log.Debug("Error processing line", log.Err(err))
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error("Scanner error", log.Err(err))
	}
}
