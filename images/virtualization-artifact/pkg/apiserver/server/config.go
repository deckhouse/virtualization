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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	"github.com/deckhouse/virtualization-controller/pkg/apiserver/api"
	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager/filesystem"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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

	// Write-capable client used by enterprise subresources (e.g. scaleDownWith) to delete pool
	// members and adjust spec.replicas from the apiserver's own identity, and to hold the console
	// and VNC session leases: it speaks both APIs, the virtualization one and Kubernetes itself.
	virtCli, err := kubeclient.GetClientFromRESTConfig(c.Rest)
	if err != nil {
		return nil, err
	}

	// Reports a session takeover on the virtual machine.
	recorder, err := newEventRecorder(virtCli)
	if err != nil {
		return nil, err
	}

	err = api.Install(vmInformer.Lister(),
		genericServer,
		c.Kubevirt,
		proxyCertManager,
		virtCli,
		recorder,
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

// newEventRecorder builds a recorder that reports events on virtual machines. The broadcaster
// lives as long as the process: the apiserver has no shutdown hook to attach it to.
func newEventRecorder(kubeCli kubeclient.Client) (record.EventRecorder, error) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		return nil, err
	}
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeCli.CoreV1().Events("")})
	return broadcaster.NewRecorder(scheme, corev1.EventSource{Component: "virtualization-api"}), nil
}
