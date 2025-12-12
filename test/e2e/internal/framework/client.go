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
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/exec"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
	dv1alpha1 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha1"
	dv1alpha2 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	gt "github.com/deckhouse/virtualization/test/e2e/internal/git"
	"github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
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
	dynamic          dynamic.Interface
	rewriteClient    rewrite.Client

	git gt.Git
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

func (c Clients) DynamicClient() dynamic.Interface {
	return c.dynamic
}

func (c Clients) RewriteClient() rewrite.Client {
	return c.rewriteClient
}

func (c Clients) Kubectl() kubectl.Kubectl {
	return c.kubectl
}

func (c Clients) D8Virtualization() d8.D8Virtualization {
	return c.d8virtualization
}

func (c Clients) Git() gt.Git {
	return c.git
}

func init() {
	onceLoadConfig()

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
	clients.dynamic, err = dynamic.NewForConfig(restConfig)
	if err != nil {
		panic(err)
	}
	clients.rewriteClient = rewrite.NewRewriteClient(clients.dynamic)
	clients.kubectl, err = kubectl.NewKubectl(kubectl.KubectlConf(conf.ClusterTransport))
	if err != nil {
		panic(err)
	}
	clients.d8virtualization, err = d8.NewD8Virtualization(d8.D8VirtualizationConf(conf.ClusterTransport))
	if err != nil {
		panic(err)
	}

	scheme := apiruntime.NewScheme()
	// virtv1 and cdiv1 are not registered in the scheme,
	// The main reason is that we cannot use kubevirt types in tests because in DVP we use rewritten kubevirt types
	// use dynamic client for get kubevirt types
	for _, f := range []func(*apiruntime.Scheme) error{
		v1alpha2.AddToScheme,
		v1alpha3.AddToScheme,
		clientgoscheme.AddToScheme,
		dv1alpha1.AddToScheme,
		dv1alpha2.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			panic(err)
		}
	}
	clients.client, err = client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	clients.git, err = gt.NewGit()
	if err != nil {
		panic(err)
	}
}
