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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type ConsoleREST struct {
	*BaseREST
}

type KubevirtAPIServerConfig struct {
	Endpoint       string
	CaBundlePath   string
	ServiceAccount types.NamespacedName
}

var (
	_ rest.Storage   = &ConsoleREST{}
	_ rest.Connecter = &ConsoleREST{}
)

func NewConsoleREST(baseREST *BaseREST) *ConsoleREST {
	return &ConsoleREST{baseREST}
}

// New implements rest.Storage interface
func (r ConsoleREST) New() runtime.Object {
	return &subresources.VirtualMachineConsole{}
}

// Destroy implements rest.Storage interface
func (r ConsoleREST) Destroy() {
}

func (r ConsoleREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	consoleOpts, ok := opts.(*subresources.VirtualMachineConsole)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	vm, err := r.vmLister.VirtualMachines(request.NamespaceValue(ctx)).Get(name)
	if err != nil {
		return nil, err
	}
	// Answered before the stream is located, so that a virtual machine that cannot be connected
	// to at all still answers that its console is free.
	if consoleOpts.Probe {
		return r.probeSession(vm, subv1alpha2.ConsoleSession), nil
	}
	location, transport, err := ConsoleLocation(ctx, r.vmLister, name, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, true, responder, r.kubevirt.ServiceAccount)
	// Connecting disconnects whoever holds the console now; record who holds it from now on,
	// so the next client can be warned before it does the same.
	return r.trackSession(vm, subv1alpha2.ConsoleSession, handler), nil
}

// NewConnectOptions implements rest.Connecter interface
func (r ConsoleREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineConsole{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r ConsoleREST) ConnectMethods() []string {
	return upgradeableMethods
}

func ConsoleLocation(
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
		newKVVMIPather("console"),
		kubevirt,
		proxyCertManager,
		virtualMachineShouldBeRunningOrMigrating,
	)
}
