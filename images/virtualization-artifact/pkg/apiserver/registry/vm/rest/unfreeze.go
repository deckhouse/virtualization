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

type UnfreezeREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &UnfreezeREST{}
	_ rest.Connecter = &UnfreezeREST{}
)

func NewUnfreezeREST(baseREST *BaseREST) *UnfreezeREST {
	return &UnfreezeREST{baseREST}
}

func (r UnfreezeREST) New() runtime.Object {
	return &subresources.VirtualMachineUnfreeze{}
}

func (r UnfreezeREST) Destroy() {
}

func (r UnfreezeREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	_, ok := opts.(*subresources.VirtualMachineUnfreeze)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := UnfreezeLocation(ctx, r.vmLister, name, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r UnfreezeREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineUnfreeze{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r UnfreezeREST) ConnectMethods() []string {
	return []string{http.MethodPut}
}

func UnfreezeLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	return streamLocation(
		ctx,
		getter,
		name,
		newKVVMIPather("unfreeze"),
		kubevirt,
		proxyCertManager,
		virtualMachineShouldBeRunning,
	)
}
