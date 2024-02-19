package server

import (
	"errors"
	"fmt"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/api"
)

var ErrConfigInvalid = errors.New("configuration is invalid")

type Config struct {
	Apiserver *genericapiserver.Config
	Rest      *rest.Config
	Kubevirt  api.KubevirtApiServerConfig
}

func (c Config) Validate() []error {
	errs := []error{}
	if c.Kubevirt.Endpoint == "" {
		errs = append(errs, fmt.Errorf(".Kubevirt.Endpoint is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.CertsPath == "" {
		errs = append(errs, fmt.Errorf(".Kubevirt.CertsPath is required. %w", ErrConfigInvalid))
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
	informer, err := informerFactory(c.Rest)
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
	if err := api.Install(vmInformer.Lister(), genericServer, c.Kubevirt); err != nil {
		return nil, err
	}
	s := NewServer(
		vmInformer.Informer(),
		genericServer,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}
