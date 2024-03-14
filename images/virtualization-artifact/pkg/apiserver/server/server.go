package server

import (
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
)

func NewServer(
	virtualMachines cache.Controller,
	apiserver *genericapiserver.GenericAPIServer,
	proxyCertManager certmanager.CertificateManager,
) *server {
	return &server{
		virtualMachines:  virtualMachines,
		GenericAPIServer: apiserver,
		proxyCertManager: proxyCertManager,
	}
}

type server struct {
	*genericapiserver.GenericAPIServer
	proxyCertManager certmanager.CertificateManager
	virtualMachines  cache.Controller
}

func (s *server) RunUntil(stopCh <-chan struct{}) error {
	go s.proxyCertManager.Start()
	// Start informers
	go s.virtualMachines.Run(stopCh)

	// Ensure cache is up-to-date
	ok := cache.WaitForCacheSync(stopCh, s.virtualMachines.HasSynced)
	if !ok {
		return nil
	}
	err := s.GenericAPIServer.PrepareRun().Run(stopCh)
	s.proxyCertManager.Stop()
	return err
}
