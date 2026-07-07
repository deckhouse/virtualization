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

package storage

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	vmpoolrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vmpool/rest"
	virtclient "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	"github.com/deckhouse/virtualization/api/subresources"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

// VirtualMachinePoolStorage is the meta-object storage for VirtualMachinePool in
// the subresources API group. Clients normally address a subresource such as
// scaleDownWith; Get is served only so the meta-object resolves for an existing
// pool, mirroring the VirtualMachine storage.
type VirtualMachinePoolStorage struct {
	client        virtclient.Interface
	scaleDownWith *vmpoolrest.ScaleDownWithREST
}

var (
	_ rest.Storage              = &VirtualMachinePoolStorage{}
	_ rest.Scoper               = &VirtualMachinePoolStorage{}
	_ rest.KindProvider         = &VirtualMachinePoolStorage{}
	_ rest.Getter               = &VirtualMachinePoolStorage{}
	_ rest.SingularNameProvider = &VirtualMachinePoolStorage{}
)

func NewStorage(c virtclient.Interface) *VirtualMachinePoolStorage {
	return &VirtualMachinePoolStorage{
		client:        c,
		scaleDownWith: vmpoolrest.NewScaleDownWithREST(c),
	}
}

func (store VirtualMachinePoolStorage) ScaleDownWithREST() *vmpoolrest.ScaleDownWithREST {
	return store.scaleDownWith
}

// New implements rest.Storage.
func (store VirtualMachinePoolStorage) New() runtime.Object {
	return &subv1alpha2.VirtualMachinePool{}
}

// Destroy implements rest.Storage.
func (store VirtualMachinePoolStorage) Destroy() {}

// Kind implements rest.KindProvider.
func (store VirtualMachinePoolStorage) Kind() string {
	return "VirtualMachinePool"
}

// NamespaceScoped implements rest.Scoper.
func (store VirtualMachinePoolStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName implements rest.SingularNameProvider.
func (store VirtualMachinePoolStorage) GetSingularName() string {
	return "virtualmachinepool"
}

// Get implements rest.Getter. Like the VirtualMachine storage, it returns a
// meta-object for a pool that exists and NotFound only when it truly does not.
func (store VirtualMachinePoolStorage) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)
	pool, err := store.client.VirtualizationV1alpha2().VirtualMachinePools(namespace).Get(ctx, name, *opts)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, k8serrors.NewNotFound(subresources.Resource("virtualmachinepools"), name)
		}
		return nil, k8serrors.NewInternalError(err)
	}
	return &subresources.VirtualMachinePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: subresources.SchemeGroupVersion.String(),
			Kind:       "VirtualMachinePool",
		},
		ObjectMeta: pool.ObjectMeta,
	}, nil
}
