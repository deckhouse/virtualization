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

package storage

import (
	"context"
	"sort"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	versionedv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineStorage struct {
	groupResource schema.GroupResource
	vmLister      virtlisters.VirtualMachineLister
	console       *vmrest.ConsoleREST
	vnc           *vmrest.VNCREST
	portforward   *vmrest.PortForwardREST
	addVolume     *vmrest.AddVolumeREST
	removeVolume  *vmrest.RemoveVolumeREST
	freeze        *vmrest.FreezeREST
	unfreeze      *vmrest.UnfreezeREST
	convertor     rest.TableConvertor
	vmClient      versionedv1alpha2.VirtualMachinesGetter
}

var (
	_ rest.KindProvider         = &VirtualMachineStorage{}
	_ rest.Storage              = &VirtualMachineStorage{}
	_ rest.Scoper               = &VirtualMachineStorage{}
	_ rest.Lister               = &VirtualMachineStorage{}
	_ rest.Getter               = &VirtualMachineStorage{}
	_ rest.GracefulDeleter      = &VirtualMachineStorage{}
	_ rest.TableConvertor       = &VirtualMachineStorage{}
	_ rest.SingularNameProvider = &VirtualMachineStorage{}
)

func NewStorage(
	groupResource schema.GroupResource,
	vmLister virtlisters.VirtualMachineLister,
	kubevirt vmrest.KubevirtApiServerConfig,
	proxyCertManager certmanager.CertificateManager,
	crd *apiextensionsv1.CustomResourceDefinition,
	vmClient versionedv1alpha2.VirtualMachinesGetter,
) *VirtualMachineStorage {
	var convertor rest.TableConvertor
	if crd != nil && len(crd.Spec.Versions) > 0 {
		newSpec := crd.Spec.DeepCopy()
		sort.Slice(newSpec.Versions, func(i, j int) bool {
			return version.CompareKubeAwareVersionStrings(newSpec.Versions[i].Name, newSpec.Versions[j].Name) > 0
		})
		for _, ver := range newSpec.Versions {
			if ver.Served && !ver.Deprecated {
				if c, err := tableconvertor.New(ver.AdditionalPrinterColumns); err == nil {
					convertor = c
					break
				}
			}
		}
	}
	return &VirtualMachineStorage{
		groupResource: groupResource,
		vmLister:      vmLister,
		console:       vmrest.NewConsoleREST(vmLister, kubevirt, proxyCertManager),
		vnc:           vmrest.NewVNCREST(vmLister, kubevirt, proxyCertManager),
		portforward:   vmrest.NewPortForwardREST(vmLister, kubevirt, proxyCertManager),
		addVolume:     vmrest.NewAddVolumeREST(vmLister, kubevirt, proxyCertManager),
		removeVolume:  vmrest.NewRemoveVolumeREST(vmLister, kubevirt, proxyCertManager),
		freeze:        vmrest.NewFreezeREST(vmLister, kubevirt, proxyCertManager),
		unfreeze:      vmrest.NewUnfreezeREST(vmLister, kubevirt, proxyCertManager),
		convertor:     convertor,
		vmClient:      vmClient,
	}
}

func (store VirtualMachineStorage) ConsoleREST() *vmrest.ConsoleREST {
	return store.console
}

func (store VirtualMachineStorage) VncREST() *vmrest.VNCREST {
	return store.vnc
}

func (store VirtualMachineStorage) PortForwardREST() *vmrest.PortForwardREST {
	return store.portforward
}

func (store VirtualMachineStorage) AddVolumeREST() *vmrest.AddVolumeREST {
	return store.addVolume
}

func (store VirtualMachineStorage) RemoveVolumeREST() *vmrest.RemoveVolumeREST {
	return store.removeVolume
}

func (store VirtualMachineStorage) FreezeREST() *vmrest.FreezeREST {
	return store.freeze
}

func (store VirtualMachineStorage) UnfreezeREST() *vmrest.UnfreezeREST {
	return store.unfreeze
}

// New implements rest.Storage interface
func (store VirtualMachineStorage) New() runtime.Object {
	return &virtv2.VirtualMachine{}
}

// Destroy implements rest.Storage interface
func (store VirtualMachineStorage) Destroy() {
}

// Kind implements rest.KindProvider interface
func (store VirtualMachineStorage) Kind() string {
	return "VirtualMachine"
}

// NamespaceScoped implements rest.Scoper interface
func (store VirtualMachineStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName implements rest.SingularNameProvider interface
func (store VirtualMachineStorage) GetSingularName() string {
	return "virtualmachine"
}

func (store VirtualMachineStorage) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)
	vm, err := store.vmLister.VirtualMachines(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, k8serrors.NewNotFound(store.groupResource, name)
		}
		return nil, k8serrors.NewInternalError(err)
	}
	return vm, nil
}

func (store VirtualMachineStorage) NewList() runtime.Object {
	return &virtv2.VirtualMachineList{}
}

func (store VirtualMachineStorage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)

	labelSelector := labels.Everything()

	var opts internalversion.ListOptions
	if options != nil {
		opts = *options
	}
	if !(opts.LabelSelector == nil || opts.LabelSelector.Empty()) {
		labelSelector = opts.LabelSelector
	}

	name, err := nameFor(opts.FieldSelector)
	if err != nil {
		return nil, err
	}

	items, err := store.vmLister.VirtualMachines(namespace).List(labelSelector)
	if err != nil {
		return nil, k8serrors.NewInternalError(err)
	}

	filtered := &virtv2.VirtualMachineList{}
	filtered.Items = make([]virtv2.VirtualMachine, 0, len(items))
	for _, vm := range items {
		if matches(vm, name) {
			filtered.Items = append(filtered.Items, *vm)
		}
	}
	return filtered, nil
}

func (store VirtualMachineStorage) ConvertToTable(ctx context.Context, object, tableOptions runtime.Object) (*metav1.Table, error) {
	if store.convertor != nil {
		return store.convertor.ConvertToTable(ctx, object, tableOptions)
	}
	return rest.NewDefaultTableConvertor(store.groupResource).ConvertToTable(ctx, object, tableOptions)
}

func (store VirtualMachineStorage) Delete(ctx context.Context, name string, _ rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	var opts metav1.DeleteOptions
	if options != nil {
		opts = *options
	}
	if err := store.vmClient.VirtualMachines(genericreq.NamespaceValue(ctx)).Delete(ctx, name, opts); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, false, k8serrors.NewNotFound(store.groupResource, name)
		}
		return nil, false, k8serrors.NewInternalError(err)
	}
	return nil, true, nil
}
