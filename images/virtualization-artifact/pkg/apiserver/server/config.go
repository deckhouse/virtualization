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
	"errors"
	"fmt"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/rest"

	"github.com/deckhouse/virtualization-controller/pkg/apiserver/api"
	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager/filesystem"
)

var ErrConfigInvalid = errors.New("configuration is invalid")

type Config struct {
	Apiserver           *genericapiserver.Config
	Rest                *rest.Config
	Kubevirt            vmrest.KubevirtAPIServerConfig
	ProxyClientCertFile string
	ProxyClientKeyFile  string
}

func (c Config) Validate() error {
	var err error
	if c.Kubevirt.Endpoint == "" {
		err = errors.Join(err, fmt.Errorf(".Kubevirt.Endpoint is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.CaBundlePath == "" {
		err = errors.Join(err, fmt.Errorf(".Kubevirt.CaBundlePath is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.ServiceAccount.Name == "" {
		err = errors.Join(err, fmt.Errorf(".Kubevirt.ServiceAccount.Name is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.ServiceAccount.Namespace == "" {
		err = errors.Join(err, fmt.Errorf(".Kubevirt.ServiceAccount.Namespace is required. %w", ErrConfigInvalid))
	}
	if c.ProxyClientCertFile == "" {
		err = errors.Join(err, fmt.Errorf(".ProxyClientCertFile is required. %w", ErrConfigInvalid))
	}
	if c.ProxyClientKeyFile == "" {
		err = errors.Join(err, fmt.Errorf(".ProxyClientKeyFile is required. %w", ErrConfigInvalid))
	}
	if c.Apiserver == nil {
		err = errors.Join(err, fmt.Errorf(".Apiserver is required. %w", ErrConfigInvalid))
	}
	if c.Rest == nil {
		err = errors.Join(err, fmt.Errorf(".Rest is required. %w", ErrConfigInvalid))
	}
	return err
}

func (c Config) Complete() (*Server, error) {
	proxyCertManager := filesystem.NewFileCertificateManager(c.ProxyClientCertFile, c.ProxyClientKeyFile)
	vmSharedInformerFactory, err := virtualizationInformerFactory(c.Rest)
	if err != nil {
		return nil, err
	}
	vmInformer := vmSharedInformerFactory.Virtualization().V1alpha2().VirtualMachines()

	genericServer, err := c.Apiserver.Complete(nil).New("virtualziation-api", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	err = api.Install(vmInformer.Lister(),
		genericServer,
		c.Kubevirt,
		proxyCertManager,
	)
	if err != nil {
		return nil, err
	}

	return NewServer(
		vmInformer.Informer(),
		genericServer,
		proxyCertManager,
	), nil
}
