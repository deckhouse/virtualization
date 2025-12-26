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

type USBRedirREST struct {
	vmLister         virtlisters.VirtualMachineLister
	proxyCertManager certmanager.CertificateManager
	kubevirt         KubevirtAPIServerConfig
}

var (
	_ rest.Storage   = &USBRedirREST{}
	_ rest.Connecter = &USBRedirREST{}
)

func NewUSBRedirREST(vmLister virtlisters.VirtualMachineLister, kubevirt KubevirtAPIServerConfig, proxyCertManager certmanager.CertificateManager) *USBRedirREST {
	return &USBRedirREST{
		vmLister:         vmLister,
		kubevirt:         kubevirt,
		proxyCertManager: proxyCertManager,
	}
}

func (r USBRedirREST) New() runtime.Object {
	return &subresources.VirtualMachineUSBRedir{}
}

// Destroy implements rest.Storage interface
func (r USBRedirREST) Destroy() {
}

func (r USBRedirREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	options, ok := opts.(*subresources.VirtualMachineUSBRedir)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := USBRedirLocation(ctx, r.vmLister, name, options, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, true, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r USBRedirREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineUSBRedir{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r USBRedirREST) ConnectMethods() []string {
	return upgradeableMethods
}

func USBRedirLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	_ *subresources.VirtualMachineUSBRedir,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	return streamLocation(
		ctx,
		getter,
		name,
		newKVVMIPather("usbredir"),
		kubevirt,
		proxyCertManager,
		virtualMachineShouldBeRunning,
	)
}
