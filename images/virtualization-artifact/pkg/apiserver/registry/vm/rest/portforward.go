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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	"github.com/deckhouse/virtualization/api/subresources"
)

type PortForwardREST struct {
	vmLister         cache.GenericLister
	proxyCertManager certmanager.CertificateManager
	kubevirt         KubevirtApiServerConfig
}

var (
	_ rest.Storage   = &PortForwardREST{}
	_ rest.Connecter = &PortForwardREST{}
)

func NewPortForwardREST(vmLister cache.GenericLister, kubevirt KubevirtApiServerConfig, proxyCertManager certmanager.CertificateManager) *PortForwardREST {
	return &PortForwardREST{
		vmLister:         vmLister,
		kubevirt:         kubevirt,
		proxyCertManager: proxyCertManager,
	}
}

// New implements rest.Storage interface
func (r PortForwardREST) New() runtime.Object {
	return &subresources.VirtualMachinePortForward{}
}

// Destroy implements rest.Storage interface
func (r PortForwardREST) Destroy() {
}

func (r PortForwardREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	options, ok := opts.(*subresources.VirtualMachinePortForward)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := PortForwardLocation(ctx, r.vmLister, name, options, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, true, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r PortForwardREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachinePortForward{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r PortForwardREST) ConnectMethods() []string {
	return upgradeableMethods
}

func PortForwardLocation(
	ctx context.Context,
	getter cache.GenericLister,
	name string,
	opts *subresources.VirtualMachinePortForward,
	kubevirt KubevirtApiServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	streamPath := buildPortForwardResourcePath(opts)
	return streamLocation(ctx, getter, name, opts, streamPath, kubevirt, proxyCertManager)
}

func buildPortForwardResourcePath(opts *subresources.VirtualMachinePortForward) string {
	resource := strings.Builder{}
	resource.WriteString("portforward/")
	resource.WriteString(strconv.Itoa(opts.Port))

	if len(opts.Protocol) > 0 {
		resource.WriteString("/")
		resource.WriteString(opts.Protocol)
	}

	return resource.String()
}
