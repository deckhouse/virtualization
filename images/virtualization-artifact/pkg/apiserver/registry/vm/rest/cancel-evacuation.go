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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

type CancelEvacuationREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &CancelEvacuationREST{}
	_ rest.Connecter = &CancelEvacuationREST{}
)

func NewCancelEvacuationREST(baseREST *BaseREST) *CancelEvacuationREST {
	return &CancelEvacuationREST{baseREST}
}

func (r CancelEvacuationREST) New() runtime.Object {
	return &subresources.VirtualMachineCancelEvacuation{}
}

func (r CancelEvacuationREST) Destroy() {
}

func (r CancelEvacuationREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	cancelEvacuationOpts, ok := opts.(*subresources.VirtualMachineCancelEvacuation)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}

	location, transport, err := CancelEvacuationRESTRESTLocation(ctx, r.vmLister, name, r.kubevirt, r.proxyCertManager, newKVVMPather("evacuatecancel"))
	if err != nil {
		return nil, err
	}
	hook, err := r.genMutateRequestHook(cancelEvacuationOpts)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, responder, r.kubevirt.ServiceAccount, hook)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r CancelEvacuationREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineCancelEvacuation{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r CancelEvacuationREST) ConnectMethods() []string {
	return []string{http.MethodPut}
}

func (r CancelEvacuationREST) genMutateRequestHook(opts *subresources.VirtualMachineCancelEvacuation) (mutateRequestHook, error) {
	newBody, err := json.Marshal(&KubevirtEvacuateCancelOptions{
		DryRun: opts.DryRun,
	})
	if err != nil {
		return nil, err
	}

	return func(req *http.Request) error {
		return rewriteBody(req, newBody)
	}, nil
}

type KubevirtEvacuateCancelOptions struct {
	metav1.TypeMeta `json:",inline"`
	DryRun          []string `json:"dryRun,omitempty" protobuf:"bytes,1,rep,name=dryRun"`
}

func CancelEvacuationRESTRESTLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	cancelEvacuationPather pather,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, cancelEvacuationPather, kubevirt, proxyCertManager)
}
