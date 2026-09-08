/*
Copyright 2026 Flant JSC

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
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

// SPICEREST proxies a single SPICE channel connection to the virtual machine.
//
// A SPICE client opens several connections per session — one per channel (main,
// display, inputs, cursor, sound, usbredir) — so a single session results in
// multiple concurrent requests to this subresource. Each one is handled
// independently.
type SPICEREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &SPICEREST{}
	_ rest.Connecter = &SPICEREST{}
)

func NewSPICEREST(baseREST *BaseREST) *SPICEREST {
	return &SPICEREST{baseREST}
}

// New implements rest.Storage interface
func (r SPICEREST) New() runtime.Object {
	return &subresources.VirtualMachineSPICE{}
}

// Destroy implements rest.Storage interface
func (r SPICEREST) Destroy() {
}

func (r SPICEREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	spiceOpts, ok := opts.(*subresources.VirtualMachineSPICE)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	vm, err := r.vmLister.VirtualMachines(request.NamespaceValue(ctx)).Get(name)
	if err != nil {
		return nil, err
	}
	if spiceOpts.Probe {
		return r.probeSession(vm, subv1alpha2.SPICESession), nil
	}
	location, transport, err := SPICELocation(ctx, r.vmLister, name, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, true, responder, r.kubevirt.ServiceAccount)
	// Every channel of a session is tracked, and they all belong to the same user, so
	// taking the session over from oneself raises no event. The last channel to connect
	// ends up holding the lease; the earlier ones see it is no longer theirs and stop
	// renewing, which is why a session is reported as free once that channel closes
	// rather than once the client is gone. Tracking is advisory, and the alternative —
	// teaching the platform which channel is the main one — buys nothing for it.
	return r.trackSession(vm, subv1alpha2.SPICESession, handler), nil
}

// NewConnectOptions implements rest.Connecter interface
func (r SPICEREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineSPICE{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r SPICEREST) ConnectMethods() []string {
	return upgradeableMethods
}

func SPICELocation(
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
		newKVVMIPather("spice"),
		kubevirt,
		proxyCertManager,
		virtualMachineShouldBeRunningOrMigrating,
	)
}
