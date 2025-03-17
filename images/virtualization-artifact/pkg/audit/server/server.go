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
	"encoding/json"
	"log/slog"
	"net"
	"time"

	"k8s.io/apiserver/pkg/apis/audit"

	"github.com/deckhouse/deckhouse/pkg/log"
)

// "k8s.io/apiserver/pkg/apis/audit"
type Server interface {
	Run(ctx context.Context, opts ...Option) error
}

type endpointer interface {
	IsMatched(*audit.Event) bool
	Log(*audit.Event) error
}

func NewServer(addr string, regs ...endpointer) (Server, error) {
	return &tcpServer{
		gracefulShutdownTimeout: 5 * time.Second,
		addr:                    addr,
		eventLoggers:            regs,
	}, nil
}

type tcpServer struct {
	gracefulShutdownTimeout time.Duration
	addr                    string
	eventLoggers            []endpointer
}

func (s *tcpServer) Run(ctx context.Context, opts ...Option) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	// Accept connections in a loop that respects context cancellation
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	log.Info("Server started", slog.String("address", s.addr))

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

type logMessage struct {
	Message string `json:"message"`
}

func (s *tcpServer) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	log.Info("New connection", slog.String("remote", conn.RemoteAddr().String()))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
			line := scanner.Bytes()

			var message logMessage
			if err := json.Unmarshal(line, &message); err != nil {
				log.Error("error parsing JSON", log.Err(err))
				continue
			}

			var event audit.Event
			if err := json.Unmarshal([]byte(message.Message), &event); err != nil {
				log.Error("Error parsing JSON", log.Err(err))
				continue
			}

			for _, eventLogger := range s.eventLoggers {
				if eventLogger.IsMatched(&event) {
					if err := eventLogger.Log(&event); err != nil {
						log.Error("fail to log event", log.Err(err))
					}
					break
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error("Scanner error", log.Err(err))
	}
}
