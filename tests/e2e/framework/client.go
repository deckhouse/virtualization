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

package framework

import (
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/kubectl"

	// register auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth/exec"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var clients = Clients{}

func GetClients() Clients {
	return clients
}

type Clients struct {
	virtClient       kubeclient.Client
	kubeClient       kubernetes.Interface
	kubectl          kubectl.Kubectl
	d8virtualization d8.D8Virtualization
	client           client.Client
}

func (c Clients) VirtClient() kubeclient.Client {
	return c.virtClient
}

func (c Clients) KubeClient() kubernetes.Interface {
	return c.kubeClient
}

func (c Clients) GenericClient() client.Client {
	return c.client
}

func (c Clients) Kubectl() kubectl.Kubectl {
	return c.kubectl
}

func (c Clients) D8Virtualization() d8.D8Virtualization {
	return c.d8virtualization
}

func init() {
	conf, err := config.GetConfig()
	if err != nil {
		panic(err)
	}
	restConfig, err := conf.ClusterTransport.RestConfig()
	if err != nil {
		panic(err)
	}
	clients.virtClient, err = kubeclient.GetClientFromRESTConfig(restConfig)
	if err != nil {
		panic(err)
	}
	clients.kubeClient, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		panic(err)
	}
	clients.kubectl, err = kubectl.NewKubectl(kubectl.KubectlConf(conf.ClusterTransport))
	if err != nil {
		panic(err)
	}
	clients.d8virtualization, err = d8.NewD8Virtualization(d8.D8VirtualizationConf(conf.ClusterTransport))
	if err != nil {
		panic(err)
	}

	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		virtv2.AddToScheme,
		virtv1.AddToScheme,
		cdiv1.AddToScheme,
		clientgoscheme.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			panic(err)
		}
	}
	clients.client, err = client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}
}
