package server

import (
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/tools/cache"
)

func NewServer(
	virtualMachines cache.Controller,
	apiserver *genericapiserver.GenericAPIServer) *server {
	return &server{
		virtualMachines:  virtualMachines,
		GenericAPIServer: apiserver,
	}
}

type server struct {
	*genericapiserver.GenericAPIServer
	virtualMachines cache.Controller
}

func (s *server) RunUntil(stopCh <-chan struct{}) error {
	// Start informers
	go s.virtualMachines.Run(stopCh)

	// Ensure cache is up to date
	ok := cache.WaitForCacheSync(stopCh, s.virtualMachines.HasSynced)
	if !ok {
		return nil
	}
	return s.GenericAPIServer.PrepareRun().Run(stopCh)
}
