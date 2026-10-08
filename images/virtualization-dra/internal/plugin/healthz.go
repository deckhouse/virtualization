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

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	drapb "k8s.io/kubelet/pkg/apis/dra/v1"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
)

// HealthCheck serves /healthz over HTTP; it answers 200 only if both the plugin registrar and the DRA socket are reachable.
type HealthCheck struct {
	server      *http.Server
	log         *slog.Logger
	wg          sync.WaitGroup
	regSockPath string
	draSockPath string
}

func NewHealthCheck(driverName, addr string) *HealthCheck {
	regSockPath := (&url.URL{
		Scheme: "unix",
		Path:   registrarSocketPath(driverName),
	}).String()

	draSockPath := (&url.URL{
		Scheme: "unix",
		Path:   pluginSocketPath(driverName),
	}).String()

	h := &HealthCheck{
		log:         slog.With(slog.String("component", "healthcheck")),
		regSockPath: regSockPath,
		draSockPath: draSockPath,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.serveHealthz)
	h.server = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	return h
}

func (h *HealthCheck) Start() error {
	addr := h.server.Addr
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen for healthcheck service at %s: %w", addr, err)
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.log.Info("starting healthcheck service", slog.String("addr", lis.Addr().String()))
		if err := h.server.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.log.Error("failed to serve healthcheck service", slog.String("addr", addr), slog.Any("err", err))
		}
	}()

	return nil
}

func (h *HealthCheck) Stop() {
	h.log.Info("stopping healthcheck service")
	if err := h.server.Close(); err != nil {
		h.log.Error("failed to stop healthcheck service", slog.Any("err", err))
	}
	h.wg.Wait()
}

func (h *HealthCheck) serveHealthz(w http.ResponseWriter, r *http.Request) {
	if err := h.check(r.Context()); err != nil {
		h.log.Error("health check failed", slog.Any("err", err))
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (h *HealthCheck) check(ctx context.Context) error {
	regConn, err := dialUnix(h.regSockPath)
	if err != nil {
		return fmt.Errorf("connect to registration socket: %w", err)
	}
	defer func() { _ = regConn.Close() }()

	info, err := registerapi.NewRegistrationClient(regConn).GetInfo(ctx, &registerapi.InfoRequest{})
	if err != nil {
		return fmt.Errorf("call GetInfo: %w", err)
	}
	h.log.Debug("Successfully invoked GetInfo", "info", info)

	draConn, err := dialUnix(h.draSockPath)
	if err != nil {
		return fmt.Errorf("connect to DRA socket: %w", err)
	}
	defer func() { _ = draConn.Close() }()

	_, err = drapb.NewDRAPluginClient(draConn).NodePrepareResources(ctx, &drapb.NodePrepareResourcesRequest{})
	if err != nil {
		return fmt.Errorf("call NodePrepareResources: %w", err)
	}
	h.log.Debug("Successfully invoked NodePrepareResources")

	return nil
}

func dialUnix(target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
