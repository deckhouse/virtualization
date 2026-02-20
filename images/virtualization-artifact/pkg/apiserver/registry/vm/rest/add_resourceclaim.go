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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

type AddResourceClaimREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &AddResourceClaimREST{}
	_ rest.Connecter = &AddResourceClaimREST{}
)

func NewAddResourceClaimREST(baseREST *BaseREST) *AddResourceClaimREST {
	return &AddResourceClaimREST{baseREST}
}

func (r AddResourceClaimREST) New() runtime.Object {
	return &subresources.VirtualMachineAddResourceClaim{}
}

func (r AddResourceClaimREST) Destroy() {
}

func (r AddResourceClaimREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	addResourceClaimOpts, ok := opts.(*subresources.VirtualMachineAddResourceClaim)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	var (
		resourceClaimPather pather
		hooks               []mutateRequestHook
	)

	if r.requestFromKubevirt(addResourceClaimOpts) {
		resourceClaimPather = newKVVMIPather("addresourceclaim")
	} else {
		resourceClaimPather = newKVVMPather("addresourceclaim")
		h, err := r.genMutateRequestHook(addResourceClaimOpts)
		if err != nil {
			return nil, err
		}
		hooks = append(hooks, h)
	}
	location, transport, err := AddResourceClaimLocation(ctx, r.vmLister, name, r.kubevirt, r.proxyCertManager, resourceClaimPather)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, responder, r.kubevirt.ServiceAccount, hooks...)

	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r AddResourceClaimREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineAddResourceClaim{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r AddResourceClaimREST) ConnectMethods() []string {
	return []string{http.MethodPut}
}

func (r AddResourceClaimREST) requestFromKubevirt(opts *subresources.VirtualMachineAddResourceClaim) bool {
	return opts == nil || (opts.Name == "" && opts.ResourceClaimTemplateName == "" && opts.RequestName == "")
}

func (r AddResourceClaimREST) genMutateRequestHook(opts *subresources.VirtualMachineAddResourceClaim) (mutateRequestHook, error) {
	hotplugRequest := virtv1.AddResourceClaimOptions{
		Name: opts.Name,
		HostDevice: &virtv1.HostDevice{
			Name: opts.Name,
			ClaimRequest: &virtv1.ClaimRequest{
				ClaimName:   ptr.To(opts.Name),
				RequestName: ptr.To(opts.RequestName),
			},
		},
		ResourceClaim: &virtv1.ResourceClaim{
			PodResourceClaim: corev1.PodResourceClaim{
				Name:                      opts.Name,
				ResourceClaimTemplateName: ptr.To(opts.ResourceClaimTemplateName),
			},
			Hotpluggable: true,
		},
		DryRun: opts.DryRun,
	}

	newBody, err := json.Marshal(&hotplugRequest)
	if err != nil {
		return nil, err
	}

	return func(req *http.Request) error {
		return rewriteBody(req, newBody)
	}, nil
}

func AddResourceClaimLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	addResourceClaimPather pather,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, addResourceClaimPather, kubevirt, proxyCertManager)
}
