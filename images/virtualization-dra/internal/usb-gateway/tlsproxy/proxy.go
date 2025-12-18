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

package tlsproxy

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"strconv"
)

type TLSProxy struct {
	tlsConfig *tls.Config
	port      int
}

func NewTLSProxy(tlsConfig *tls.Config, port int) *TLSProxy {
	return &TLSProxy{
		tlsConfig: tlsConfig,
		port:      port,
	}
}

func (p *TLSProxy) Start(ctx context.Context, plainConn net.Conn) error {
	listener, err := tls.Listen("tcp", net.JoinHostPort("", strconv.Itoa(p.port)), p.tlsConfig)
	if err != nil {
		return err
	}

	go func() {
		defer listener.Close()
		defer plainConn.Close()

		tlsConn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				slog.Error("accept error", slog.Any("error", err))
				return
			}
		}
		defer tlsConn.Close()

		done := make(chan struct{}, 2)

		// TLS -> plain
		go func() {
			_, err = io.Copy(plainConn, tlsConn)
			if err != nil {
				slog.Error("copy error from TLS to plain",
					slog.Any("error", err),
					slog.String("tlsRemoteAddr", tlsConn.RemoteAddr().String()),
					slog.String("proxyRemoteAddr", plainConn.RemoteAddr().String()),
				)
			}
			done <- struct{}{}
		}()

		// plain -> TLS
		go func() {
			_, err = io.Copy(tlsConn, plainConn)
			if err != nil {
				slog.Error("copy error from plain to TLS",
					slog.Any("error", err),
					slog.String("tlsRemoteAddr", tlsConn.RemoteAddr().String()),
					slog.String("proxyRemoteAddr", plainConn.RemoteAddr().String()),
				)
			}
			done <- struct{}{}
		}()

		select {
		case <-ctx.Done():
		case <-done:
		}
	}()

	return nil
}
