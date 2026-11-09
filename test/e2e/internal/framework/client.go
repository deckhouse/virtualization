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
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/exec"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
	dv1alpha1 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha1"
	dv1alpha2 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	gt "github.com/deckhouse/virtualization/test/e2e/internal/git"
	"github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

// ControllerServiceAccountUser is the user the module's controller acts as. Writes to the
// internal API group are only admitted for it and its siblings.
const ControllerServiceAccountUser = "system:serviceaccount:d8-virtualization:virtualization-controller"

const (
	dialRetryWindow   = 2 * time.Minute
	dialRetryInterval = 5 * time.Second
)

// retryOnDialError repeats a request when the connection to the API server fails.
// Such a request did not reach the server, so a repeat is safe for every verb.
// The network between the CI runner and the cluster drops for some seconds,
// and one failed request must not fail a spec or the whole suite.
type retryOnDialError struct {
	next     http.RoundTripper
	window   time.Duration
	interval time.Duration
}

func (t retryOnDialError) RoundTrip(req *http.Request) (*http.Response, error) {
	deadline := time.Now().Add(t.window)
	resp, err := t.next.RoundTrip(req)
	for attempt := 2; isDialError(err) && time.Now().Before(deadline); attempt++ {
		// The transport closes a sent body. Without GetBody it cannot be sent again.
		if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
			return resp, err
		}
		select {
		case <-req.Context().Done():
			return resp, err
		case <-time.After(t.interval):
		}

		retry := req.Clone(req.Context())
		if req.GetBody != nil {
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				return resp, err
			}
			retry.Body = body
		}
		// The log line shows in the report which requests were repeated.
		GinkgoWriter.Printf("retryOnDialError: %s %s, attempt %d after: %v\n", req.Method, req.URL.Path, attempt, err)
		resp, err = t.next.RoundTrip(retry)
	}
	return resp, err
}

func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

var clients = Clients{}

var clientsOnce sync.Once

func GetClients() Clients {
	onceLoadConfig()
	InitClients()
	return clients
}

type Clients struct {
	restConfig         *rest.Config
	scheme             *apiruntime.Scheme
	virtClient         kubeclient.Client
	kubeClient         kubernetes.Interface
	kubectl            kubectl.Kubectl
	d8virtualization   d8.D8Virtualization
	client             client.Client
	controllerSAClient client.Client
	dynamic            dynamic.Interface
	rewriteClient      rewrite.Client

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

// ControllerSAClient acts as the virtualization-controller ServiceAccount. The internal
// API group is closed by the virtualization-restricted-access-policy admission policy,
// which only admits the module's own ServiceAccounts, so tests that have to write an
// internal resource (KVVM, KVVMI) go through this client.
func (c Clients) ControllerSAClient() client.Client {
	return c.controllerSAClient
}

// ImpersonatedClient acts as the given user and groups: for checks of what the admission policies admit
// to an identity the test cannot log in as.
func (c Clients) ImpersonatedClient(user string, groups ...string) (client.Client, error) {
	cfg := rest.CopyConfig(c.restConfig)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: user, Groups: groups}
	return client.New(cfg, client.Options{Scheme: c.scheme})
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

// InitClients initializes Kubernetes clients.
// This should be called before using framework clients.
func InitClients() {
	clientsOnce.Do(func() {
		onceLoadConfig()

		restConfig, err := conf.ClusterTransport.RestConfig()
		if err != nil {
			panic(err)
		}
		restConfig.Wrap(func(rt http.RoundTripper) http.RoundTripper {
			return retryOnDialError{next: rt, window: dialRetryWindow, interval: dialRetryInterval}
		})
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
			intvirtv1alpha1.AddToScheme,
			clientgoscheme.AddToScheme,
			dv1alpha1.AddToScheme,
			dv1alpha2.AddToScheme,
			sdnapi.AddToScheme,
		} {
			if err := f(scheme); err != nil {
				panic(err)
			}
		}
		clients.client, err = client.New(restConfig, client.Options{Scheme: scheme})
		if err != nil {
			panic(err)
		}
		clients.restConfig, clients.scheme = restConfig, scheme

		controllerSAConfig := rest.CopyConfig(restConfig)
		controllerSAConfig.Impersonate = rest.ImpersonationConfig{
			UserName: ControllerServiceAccountUser,
		}
		clients.controllerSAClient, err = client.New(controllerSAConfig, client.Options{Scheme: scheme})
		if err != nil {
			panic(err)
		}

		clients.git, err = gt.NewGit()
		if err != nil {
			panic(err)
		}
	})
}
