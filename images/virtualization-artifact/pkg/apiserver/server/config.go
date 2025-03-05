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
	"fmt"
	"os"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/apiserver/api"
	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager/filesystem"
	virtClient "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var ErrConfigInvalid = errors.New("configuration is invalid")

type Config struct {
	Apiserver           *genericapiserver.Config
	Rest                *rest.Config
	Kubevirt            vmrest.KubevirtApiServerConfig
	ProxyClientCertFile string
	ProxyClientKeyFile  string

	KubevirtClientKubeconfig string
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
	if c.KubevirtClientKubeconfig == "" {
		err = errors.Join(err, fmt.Errorf(".KubevirtClientKubeconfig is required. %w", ErrConfigInvalid))
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

	kubeclient, err := apiextensionsv1.NewForConfig(c.Rest)
	if err != nil {
		return nil, err
	}
	crd, err := kubeclient.CustomResourceDefinitions().Get(context.Background(), virtv2.Resource(virtv2.VirtualMachineResource).String(), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	virtclient, err := virtClient.NewForConfig(c.Rest)
	if err != nil {
		return nil, err
	}
	kubevirtClient, err := c.kubevirtClient()
	if err != nil {
		return nil, err
	}

	if err = api.Install(vmInformer.Lister(),
		genericServer,
		c.Kubevirt,
		proxyCertManager,
		crd,
		virtclient.VirtualizationV1alpha2(),
		kubevirtClient,
	); err != nil {
		return nil, err
	}

	return NewServer(
		vmInformer.Informer(),
		genericServer,
		proxyCertManager,
	), nil
}

func (c *Config) kubevirtClient() (client.Client, error) {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		virtv1.AddToScheme,
	} {
		err := f(scheme)
		if err != nil {
			return nil, err
		}
	}

	b, err := os.ReadFile(c.KubevirtClientKubeconfig)
	if err != nil {
		return nil, err
	}
	clientCfg, err := clientcmd.NewClientConfigFromBytes(b)
	if err != nil {
		return nil, err
	}
	cfg, err := clientCfg.ClientConfig()
	if err != nil {
		return nil, err
	}
	cfg.ContentType = apiruntime.ContentTypeJSON
	cfg.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()

	return client.New(cfg, client.Options{Scheme: scheme})
}
