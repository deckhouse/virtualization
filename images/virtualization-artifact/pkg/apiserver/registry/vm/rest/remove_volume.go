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
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	"github.com/deckhouse/virtualization/api/subresources"
)

type RemoveVolumeREST struct {
	vmLister         cache.GenericLister
	proxyCertManager certmanager.CertificateManager
	kubevirt         KubevirtApiServerConfig
}

var (
	_ rest.Storage   = &RemoveVolumeREST{}
	_ rest.Connecter = &RemoveVolumeREST{}
)

func NewRemoveVolumeREST(vmLister cache.GenericLister, kubevirt KubevirtApiServerConfig, proxyCertManager certmanager.CertificateManager) *RemoveVolumeREST {
	return &RemoveVolumeREST{
		vmLister:         vmLister,
		kubevirt:         kubevirt,
		proxyCertManager: proxyCertManager,
	}
}

func (r RemoveVolumeREST) New() runtime.Object {
	return &subresources.VirtualMachineRemoveVolume{}
}

func (r RemoveVolumeREST) Destroy() {
}

func (r RemoveVolumeREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	removeVolumeOpts, ok := opts.(*subresources.VirtualMachineRemoveVolume)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := RemoveVolumeRESTLocation(ctx, r.vmLister, name, removeVolumeOpts, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r RemoveVolumeREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineRemoveVolume{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r RemoveVolumeREST) ConnectMethods() []string {
	return []string{http.MethodPut}
}

func RemoveVolumeRESTLocation(
	ctx context.Context,
	getter cache.GenericLister,
	name string,
	opts *subresources.VirtualMachineRemoveVolume,
	kubevirt KubevirtApiServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, opts, "removevolume", kubevirt, proxyCertManager)
}
