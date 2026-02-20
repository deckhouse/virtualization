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
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	drapb "k8s.io/kubelet/pkg/apis/dra/v1"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
)

// HealthCheck implements the gRPC health check; reports SERVING only if both the plugin registrar and the DRA socket are reachable (for liveness).
type HealthCheck struct {
	grpc_health_v1.UnimplementedHealthServer
	server      *grpc.Server
	log         *slog.Logger
	wg          sync.WaitGroup
	regSockPath string
	draSockPath string
	port        int
}

func NewHealthCheck(driverName string, port int) *HealthCheck {
	regSockPath := (&url.URL{
		Scheme: "unix",
		Path:   registrarSocketPath(driverName),
	}).String()

	draSockPath := (&url.URL{
		Scheme: "unix",
		Path:   pluginSocketPath(driverName),
	}).String()

	h := &HealthCheck{
		server:      grpc.NewServer(),
		log:         slog.With(slog.String("component", "healthcheck")),
		regSockPath: regSockPath,
		draSockPath: draSockPath,
		port:        port,
	}
	grpc_health_v1.RegisterHealthServer(h.server, h)

	return h
}

func (h *HealthCheck) Start() error {
	addr := net.JoinHostPort("", strconv.Itoa(h.port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen for healthcheck service at %s: %w", addr, err)
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.log.Info("starting healthcheck service", slog.String("addr", lis.Addr().String()))
		if err := h.server.Serve(lis); err != nil {
			h.log.Error("failed to serve healthcheck service", slog.String("addr", addr), slog.Any("err", err))
		}
	}()

	return nil
}

func (h *HealthCheck) Stop() {
	if h.server != nil {
		h.log.Info("stopping healthcheck service")
		h.server.GracefulStop()
	}
	h.wg.Wait()
}

// Check implements [grpc_health_v1.HealthServer].
func (h *HealthCheck) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	knownServices := map[string]struct{}{"": {}, "liveness": {}}
	if _, known := knownServices[req.GetService()]; !known {
		return nil, status.Error(codes.NotFound, "unknown service")
	}

	healthCheckResponse := &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
	}

	regClient, err := h.newRegClient()
	if err != nil {
		h.log.Error("failed to create registration client", slog.Any("err", err))
		return healthCheckResponse, err
	}

	info, err := regClient.GetInfo(ctx, &registerapi.InfoRequest{})
	if err != nil {
		h.log.Error("failed to call GetInfo", slog.Any("err", err))
		return healthCheckResponse, nil
	}
	h.log.Debug("Successfully invoked GetInfo", "info", info)

	draClient, err := h.newDraConn()
	if err != nil {
		h.log.Error("failed to create DRA client", slog.Any("err", err))
		return healthCheckResponse, err
	}

	_, err = draClient.NodePrepareResources(ctx, &drapb.NodePrepareResourcesRequest{})
	if err != nil {
		h.log.Error("failed to call NodePrepareResources", slog.Any("err", err))
		return healthCheckResponse, nil
	}
	h.log.Debug("Successfully invoked NodePrepareResources")

	healthCheckResponse.Status = grpc_health_v1.HealthCheckResponse_SERVING
	return healthCheckResponse, nil
}

func (h *HealthCheck) newRegClient() (registerapi.RegistrationClient, error) {
	regConn, err := grpc.NewClient(
		h.regSockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to registration socket: %w", err)
	}
	return registerapi.NewRegistrationClient(regConn), nil
}

func (h *HealthCheck) newDraConn() (drapb.DRAPluginClient, error) {
	draConn, err := grpc.NewClient(
		h.draSockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to DRA socket: %w", err)
	}
	return drapb.NewDRAPluginClient(draConn), nil
}
