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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

type VNCREST struct {
	vmLister         virtlisters.VirtualMachineLister
	proxyCertManager certmanager.CertificateManager
	kubevirt         KubevirtAPIServerConfig
}

var (
	_ rest.Storage   = &VNCREST{}
	_ rest.Connecter = &VNCREST{}
)

func NewVNCREST(vmLister virtlisters.VirtualMachineLister, kubevirt KubevirtAPIServerConfig, proxyCertManager certmanager.CertificateManager) *VNCREST {
	return &VNCREST{
		vmLister:         vmLister,
		kubevirt:         kubevirt,
		proxyCertManager: proxyCertManager,
	}
}

// New implements rest.Storage interface
func (r VNCREST) New() runtime.Object {
	return &subresources.VirtualMachineVNC{}
}

// Destroy implements rest.Storage interface
func (r VNCREST) Destroy() {
}

func (r VNCREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	vncOpts, ok := opts.(*subresources.VirtualMachineVNC)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := VNCLocation(ctx, r.vmLister, name, vncOpts, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, true, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r VNCREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineVNC{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r VNCREST) ConnectMethods() []string {
	return upgradeableMethods
}

func VNCLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	opts *subresources.VirtualMachineVNC,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	return streamLocation(
		ctx,
		getter,
		name,
		newKVVMIPather("vnc"),
		kubevirt,
		proxyCertManager,
		virtualMachineNeedRunning,
	)
}
