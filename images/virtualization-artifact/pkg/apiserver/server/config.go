package server

import (
	"errors"
	"fmt"


	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"

	virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/api"
	rest2 "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certManager/filesystem"
)

var ErrConfigInvalid = errors.New("configuration is invalid")

type Config struct {
	Apiserver           *genericapiserver.Config
	Rest                *rest.Config
	Kubevirt            rest2.KubevirtApiServerConfig
	ProxyClientCertFile string
	ProxyClientKeyFile  string
}

func (c Config) Validate() []error {
	errs := []error{}
	if c.Kubevirt.Endpoint == "" {
		errs = append(errs, fmt.Errorf(".Kubevirt.Endpoint is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.CaBundlePath == "" {
		errs = append(errs, fmt.Errorf(".Kubevirt.CaBundlePath is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.ServiceAccount.Name == "" {
		errs = append(errs, fmt.Errorf(".Kubevirt.ServiceAccount.Name is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.ServiceAccount.Namespace == "" {
		errs = append(errs, fmt.Errorf(".Kubevirt.ServiceAccount.Namespace is required. %w", ErrConfigInvalid))
	}
	if c.ProxyClientCertFile == "" {
		errs = append(errs, fmt.Errorf(".ProxyClientCertFile is required. %w", ErrConfigInvalid))
	}
	if c.ProxyClientKeyFile == "" {
		errs = append(errs, fmt.Errorf(".ProxyClientKeyFile is required. %w", ErrConfigInvalid))
	}
	if c.Apiserver == nil {
		errs = append(errs, fmt.Errorf(".Apiserver is required. %w", ErrConfigInvalid))
	}
	if c.Rest == nil {
		errs = append(errs, fmt.Errorf(".Rest is required. %w", ErrConfigInvalid))
	}
	return errs
}

func (c Config) Complete() (*server, error) {
	proxyCertManager := filesystem.NewFileCertificateManager(c.ProxyClientCertFile, c.ProxyClientKeyFile)
	informer, err := virtualizationInformerFactory(c.Rest)
	if err != nil {
		return nil, err
	}
	vmInformer, err := informer.ForResource(virtv2.GroupVersionResource(virtv2.VMResource))
	if err != nil {
		return nil, err
	}

	genericServer, err := c.Apiserver.Complete(nil).New("virtualziation-api", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}
	if err := api.Install(vmInformer.Lister(), genericServer, c.Kubevirt, proxyCertManager); err != nil {
		return nil, err
	}

	s := NewServer(
		vmInformer.Informer(),
		genericServer,
		proxyCertManager,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}
