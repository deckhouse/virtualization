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

type RemoveVolumeREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &RemoveVolumeREST{}
	_ rest.Connecter = &RemoveVolumeREST{}
)

func NewRemoveVolumeREST(baseREST *BaseREST) *RemoveVolumeREST {
	return &RemoveVolumeREST{baseREST}
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
	var (
		removeVolumePather pather
		hooks              []mutateRequestHook
	)

	if r.requestFromKubevirt(removeVolumeOpts) {
		removeVolumePather = newKVVMIPather("removevolume")
	} else {
		removeVolumePather = newKVVMPather("removevolume")
		h, err := r.genMutateRequestHook(removeVolumeOpts)
		if err != nil {
			return nil, err
		}
		hooks = append(hooks, h)
	}

	location, transport, err := RemoveVolumeRESTLocation(ctx, r.vmLister, name, removeVolumeOpts, r.kubevirt, r.proxyCertManager, removeVolumePather)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, responder, r.kubevirt.ServiceAccount, hooks...)
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

func (r RemoveVolumeREST) requestFromKubevirt(opts *subresources.VirtualMachineRemoveVolume) bool {
	return opts == nil || opts.Name == ""
}

func (r RemoveVolumeREST) genMutateRequestHook(opts *subresources.VirtualMachineRemoveVolume) (mutateRequestHook, error) {
	unplugRequest := virtv1.RemoveVolumeOptions{
		Name: opts.Name,
	}

	newBody, err := json.Marshal(&unplugRequest)
	if err != nil {
		return nil, err
	}

	return func(req *http.Request) error {
		return rewriteBody(req, newBody)
	}, nil
}

func RemoveVolumeRESTLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	opts *subresources.VirtualMachineRemoveVolume,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	removeVolumePather pather,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, removeVolumePather, kubevirt, proxyCertManager)
}
