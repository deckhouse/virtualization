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

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

type PortForwardREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &PortForwardREST{}
	_ rest.Connecter = &PortForwardREST{}
)

func NewPortForwardREST(baseREST *BaseREST) *PortForwardREST {
	return &PortForwardREST{baseREST}
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
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, true, responder, r.kubevirt.ServiceAccount)
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
	getter virtlisters.VirtualMachineLister,
	name string,
	opts *subresources.VirtualMachinePortForward,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	streamPath := buildPortForwardResourcePath(opts)
	return streamLocation(
		ctx,
		getter,
		name,
		newKVVMIPather(streamPath),
		kubevirt,
		proxyCertManager,
		virtualMachineShouldBeRunningOrMigrating,
	)
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
