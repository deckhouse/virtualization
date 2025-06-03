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
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	userHeader    = "X-Remote-User"
	groupHeader   = "X-Remote-Group"
	kvvmiPathTmpl = "/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/%s"
	kvvmPathTmpl  = "/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachines/%s/%s"
)

func newKVVMIPather(subresource string) pather {
	return pather{
		template:    kvvmiPathTmpl,
		subresource: subresource,
	}
}

func newKVVMPather(subresource string) pather {
	return pather{
		template:    kvvmPathTmpl,
		subresource: subresource,
	}
}

type pather struct {
	template    string
	subresource string
}

func (p pather) Path(namespace, name string) string {
	return fmt.Sprintf(p.template, namespace, name, p.subresource)
}

type preconditionVirtualMachine func(vm *virtv2.VirtualMachine) error

func virtualMachineNeedRunning(vm *virtv2.VirtualMachine) error {
	if vm == nil || vm.Status.Phase != virtv2.MachineRunning {
		return fmt.Errorf("VirtualMachine is not Running")
	}
	return nil
}

var upgradeableMethods = []string{http.MethodGet, http.MethodPost}

func streamLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	pather pather,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	preConditions ...preconditionVirtualMachine,
) (*url.URL, *http.Transport, error) {
	ns, _ := request.NamespaceFrom(ctx)
	vm, err := getter.VirtualMachines(ns).Get(name)
	if err != nil {
		return nil, nil, err
	}

	for _, preCond := range preConditions {
		if err = preCond(vm); err != nil {
			return nil, nil, err
		}
	}

	location := &url.URL{
		Scheme:   "https",
		Host:     kubevirt.Endpoint,
		Path:     pather.Path(vm.Namespace, name),
		RawQuery: url.Values{}.Encode(),
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

type mutateRequestHook func(req *http.Request) error

func newThrottledUpgradeAwareProxyHandler(
	location *url.URL,
	transport *http.Transport,
	upgradeRequired bool,
	responder rest.Responder,
	sa types.NamespacedName,
	mutateHooks ...mutateRequestHook,
) http.Handler {
	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		r.Header.Add(userHeader, fmt.Sprintf("system:serviceaccount:%s:%s", sa.Namespace, sa.Name))
		r.Header.Add(groupHeader, "system:serviceaccounts")
		for _, hook := range mutateHooks {
			if hook != nil {
				if err := hook(r); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(err.Error()))
					return
				}
			}
		}
		proxyHandler := proxy.NewUpgradeAwareHandler(location, transport, false, upgradeRequired, proxy.NewErrorResponder(responder))
		proxyHandler.ServeHTTP(w, r)
	}
	return handler
}

func rewriteBody(req *http.Request, newBody []byte) error {
	if req.Body != nil {
		err := req.Body.Close()
		if err != nil {
			return err
		}
	}
	req.Body = io.NopCloser(bytes.NewBuffer(newBody))
	req.ContentLength = int64(len(newBody))
	return nil
}
