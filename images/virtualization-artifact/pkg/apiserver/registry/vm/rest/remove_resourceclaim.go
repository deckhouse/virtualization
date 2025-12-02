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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

type RemoveResourceClaimREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &RemoveResourceClaimREST{}
	_ rest.Connecter = &RemoveResourceClaimREST{}
)

func NewRemoveResourceClaimREST(baseREST *BaseREST) *RemoveResourceClaimREST {
	return &RemoveResourceClaimREST{baseREST}
}

func (r RemoveResourceClaimREST) New() runtime.Object {
	return &subresources.VirtualMachineRemoveResourceClaim{}
}

func (r RemoveResourceClaimREST) Destroy() {
}

func (r RemoveResourceClaimREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	removeResourceClaimOpts, ok := opts.(*subresources.VirtualMachineRemoveResourceClaim)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	var (
		removeResourceClaimPather pather
		hooks                     []mutateRequestHook
	)

	if r.requestFromKubevirt(removeResourceClaimOpts) {
		removeResourceClaimPather = newKVVMIPather("removeresourceclaim")
	} else {
		removeResourceClaimPather = newKVVMPather("removeresourceclaim")
		h, err := r.genMutateRequestHook(removeResourceClaimOpts)
		if err != nil {
			return nil, err
		}
		hooks = append(hooks, h)
	}

	location, transport, err := RemoveResourceClaimRESTLocation(ctx, r.vmLister, name, r.kubevirt, r.proxyCertManager, removeResourceClaimPather)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, responder, r.kubevirt.ServiceAccount, hooks...)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r RemoveResourceClaimREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineRemoveResourceClaim{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r RemoveResourceClaimREST) ConnectMethods() []string {
	return []string{http.MethodPut}
}

func (r RemoveResourceClaimREST) requestFromKubevirt(opts *subresources.VirtualMachineRemoveResourceClaim) bool {
	return opts == nil || opts.Name == ""
}

func (r RemoveResourceClaimREST) genMutateRequestHook(opts *subresources.VirtualMachineRemoveResourceClaim) (mutateRequestHook, error) {
	unplugRequest := virtv1.RemoveResourceClaimOptions{
		Name:   opts.Name,
		DryRun: opts.DryRun,
	}

	newBody, err := json.Marshal(&unplugRequest)
	if err != nil {
		return nil, err
	}

	return func(req *http.Request) error {
		return rewriteBody(req, newBody)
	}, nil
}

func RemoveResourceClaimRESTLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	removeResourceClaimPather pather,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, removeResourceClaimPather, kubevirt, proxyCertManager)
}
