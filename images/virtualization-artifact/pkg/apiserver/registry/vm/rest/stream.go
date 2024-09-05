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

package rest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

const (
	userHeader       = "X-Remote-User"
	groupHeader      = "X-Remote-Group"
	kubevirtPathTmpl = "/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/%s"
)

var upgradeableMethods = []string{http.MethodGet, http.MethodPost}

func streamLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	opts runtime.Object,
	streamPath string,
	kubevirt KubevirtApiServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	ns, _ := request.NamespaceFrom(ctx)
	vm, err := getVM(getter, types.NamespacedName{Namespace: ns, Name: name})
	if err != nil {
		return nil, nil, err
	}

	if vm.Status.Phase != virtv2.MachineRunning {
		return nil, nil, fmt.Errorf("VirtualMachine is not Running")
	}

	params := url.Values{}
	if err := streamParams(params, opts); err != nil {
		return nil, nil, err
	}

	location := &url.URL{
		Scheme:   "https",
		Host:     kubevirt.Endpoint,
		Path:     fmt.Sprintf(kubevirtPathTmpl, vm.Namespace, name, streamPath),
		RawQuery: params.Encode(),
	}
	ca, err := os.ReadFile(kubevirt.CaBundlePath)
	if err != nil {
		return nil, nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(ca)

	cert := proxyCertManager.Current()
	if cert == nil {
		return nil, nil, fmt.Errorf("proxy certificate not found")
	}
	tlsConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{*cert},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return location, transport, nil
}

func getVM(getter virtlisters.VirtualMachineLister, key types.NamespacedName) (*virtv2.VirtualMachine, error) {
	vm, err := getter.VirtualMachines(key.Namespace).Get(key.Name)
	return vm, err
}

// TODO: This may be useful in the future
func streamParams(_ url.Values, opts runtime.Object) error {
	switch opts := opts.(type) {
	case *subresources.VirtualMachineConsole:
		return nil
	case *subresources.VirtualMachineVNC:
		return nil
	case *subresources.VirtualMachinePortForward:
		return nil
	case *subresources.VirtualMachineAddVolume:
		return nil
	case *subresources.VirtualMachineRemoveVolume:
		return nil
	case *subresources.VirtualMachineFreeze:
		return nil
	case *subresources.VirtualMachineUnfreeze:
		return nil
	default:
		return fmt.Errorf("unknown object for streaming: %v", opts)
	}
}

func newThrottledUpgradeAwareProxyHandler(
	location *url.URL,
	transport *http.Transport,
	upgradeRequired bool,
	responder rest.Responder,
	sa types.NamespacedName,
) http.Handler {
	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		r.Header.Add(userHeader, fmt.Sprintf("system:serviceaccount:%s:%s", sa.Namespace, sa.Name))
		r.Header.Add(groupHeader, "system:serviceaccounts")
		proxyHandler := proxy.NewUpgradeAwareHandler(location, transport, false, upgradeRequired, proxy.NewErrorResponder(responder))
		proxyHandler.ServeHTTP(w, r)
	}
	return handler
}
