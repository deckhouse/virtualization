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
	"net/http"
	"time"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/client-go/kubernetes"

	"vm-route-forge/internal/server/healthz"
)

type Server struct {
	address          string
	client           kubernetes.Interface
	restfulContainer containerInterface
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.restfulContainer.ServeHTTP(writer, request)
}

func NewServer(addr string, client kubernetes.Interface) *Server {
	server := &Server{
		address:          addr,
		client:           client,
		restfulContainer: &filteringContainer{Container: restful.NewContainer()},
	}
	return server
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	server := &http.Server{
		Addr:    s.address,
		Handler: s,
	}
	errCh := make(chan error)
	go func() {
		errCh <- server.ListenAndServe()
	}()
loop:
	for {
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		case <-ctx.Done():
			break loop
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func (s *Server) InstallDefaultHandlers() {
	var reg register
	reg = healthz.NewHandler(s.client)
	s.RegisterWebService(reg.WebService())
}

func (s *Server) RegisterWebService(ws *restful.WebService) {
	s.restfulContainer.Add(ws)
}

// containerInterface defines the restful.Container functions used on the root container
type containerInterface interface {
	Add(service *restful.WebService) *restful.Container
	Handle(path string, handler http.Handler)
	Filter(filter restful.FilterFunction)
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	RegisteredWebServices() []*restful.WebService
}

// filteringContainer delegates all Handle(...) calls to Container.HandleWithFilter(...),
// so we can ensure restful.FilterFunctions are used for all handlers
type filteringContainer struct {
	*restful.Container
}

func (c *filteringContainer) Handle(path string, handler http.Handler) {
	c.HandleWithFilter(path, handler)
}

type register interface {
	WebService() *restful.WebService
}
