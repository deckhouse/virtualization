package server

import (
	"net/http"

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

func (s *Server) ListenAndServe() error {
	server := &http.Server{
		Addr:    s.address,
		Handler: s,
	}
	return server.ListenAndServe()
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
