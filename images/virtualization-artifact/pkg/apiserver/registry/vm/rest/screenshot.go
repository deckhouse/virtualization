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
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

type VNCScreenshotREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &VNCScreenshotREST{}
	_ rest.Connecter = &VNCScreenshotREST{}
)

func NewVNCScreenshotREST(baseREST *BaseREST) *VNCScreenshotREST {
	return &VNCScreenshotREST{baseREST}
}

// New implements rest.Storage interface
func (r VNCScreenshotREST) New() runtime.Object {
	return &subresources.VirtualMachineVNCScreenshot{}
}

// Destroy implements rest.Storage interface
func (r VNCScreenshotREST) Destroy() {
}

func (r VNCScreenshotREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	options, ok := opts.(*subresources.VirtualMachineVNCScreenshot)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := VNCScreenshotLocation(ctx, r.vmLister, name, options, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	// The screenshot endpoint returns a plain image/png over a regular GET, so upgrade is not required.
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r VNCScreenshotREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineVNCScreenshot{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r VNCScreenshotREST) ConnectMethods() []string {
	return []string{http.MethodGet}
}

func VNCScreenshotLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	opts *subresources.VirtualMachineVNCScreenshot,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	location, transport, err := streamLocation(
		ctx,
		getter,
		name,
		newKVVMIPather("vnc/screenshot"),
		kubevirt,
		proxyCertManager,
		virtualMachineShouldBeRunningOrMigrating,
	)
	if err != nil {
		return nil, nil, err
	}
	location.RawQuery = url.Values{"moveCursor": {strconv.FormatBool(opts.MoveCursor)}}.Encode()
	return location, transport, nil
}
