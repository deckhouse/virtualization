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

type UsbRedirREST struct {
	vmLister         virtlisters.VirtualMachineLister
	proxyCertManager certmanager.CertificateManager
	kubevirt         KubevirtAPIServerConfig
}

var (
	_ rest.Storage   = &UsbRedirREST{}
	_ rest.Connecter = &UsbRedirREST{}
)

func NewUsbRedirREST(vmLister virtlisters.VirtualMachineLister, kubevirt KubevirtAPIServerConfig, proxyCertManager certmanager.CertificateManager) *UsbRedirREST {
	return &UsbRedirREST{
		vmLister:         vmLister,
		kubevirt:         kubevirt,
		proxyCertManager: proxyCertManager,
	}
}

// New implements rest.Storage interface
func (r UsbRedirREST) New() runtime.Object {
	return &subresources.VirtualMachineUsbRedir{}
}

// Destroy implements rest.Storage interface
func (r UsbRedirREST) Destroy() {
}

func (r UsbRedirREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	usbRedirOpts, ok := opts.(*subresources.VirtualMachineUsbRedir)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := UsbRedirLocation(ctx, r.vmLister, name, usbRedirOpts, r.kubevirt, r.proxyCertManager)
	transport.ReadBufferSize = 32 * 1024
	transport.WriteBufferSize = 32 * 1024
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, true, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r UsbRedirREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineUsbRedir{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r UsbRedirREST) ConnectMethods() []string {
	return upgradeableMethods
}

func UsbRedirLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	opts *subresources.VirtualMachineUsbRedir,
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
		virtualMachineNeedRunning,
	)
}
