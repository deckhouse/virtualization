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

package storage

import (
	"context"
	"fmt"
	"sort"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource/tableconvertor"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	versionedv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

type LegacyVirtualMachineStorage struct {
	vmLister         virtlisters.VirtualMachineLister
	console          *vmrest.ConsoleREST
	vnc              *vmrest.VNCREST
	portforward      *vmrest.PortForwardREST
	addVolume        *vmrest.AddVolumeREST
	removeVolume     *vmrest.RemoveVolumeREST
	freeze           *vmrest.FreezeREST
	unfreeze         *vmrest.UnfreezeREST
	cancelEvacuation *vmrest.CancelEvacuationREST
	convertor        rest.TableConvertor
	vmClient         versionedv1alpha2.VirtualMachinesGetter
}

var (
	_ rest.KindProvider         = &LegacyVirtualMachineStorage{}
	_ rest.Storage              = &LegacyVirtualMachineStorage{}
	_ rest.Scoper               = &LegacyVirtualMachineStorage{}
	_ rest.Lister               = &LegacyVirtualMachineStorage{}
	_ rest.Getter               = &LegacyVirtualMachineStorage{}
	_ rest.GracefulDeleter      = &LegacyVirtualMachineStorage{}
	_ rest.TableConvertor       = &LegacyVirtualMachineStorage{}
	_ rest.SingularNameProvider = &LegacyVirtualMachineStorage{}
)

func NewLegacyStorage(
	vmLister virtlisters.VirtualMachineLister,
	kubevirt vmrest.KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	crd *apiextensionsv1.CustomResourceDefinition,
	vmClient versionedv1alpha2.VirtualMachinesGetter,
) *LegacyVirtualMachineStorage {
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
	return &LegacyVirtualMachineStorage{
		vmLister:         vmLister,
		console:          vmrest.NewConsoleREST(vmLister, kubevirt, proxyCertManager),
		vnc:              vmrest.NewVNCREST(vmLister, kubevirt, proxyCertManager),
		portforward:      vmrest.NewPortForwardREST(vmLister, kubevirt, proxyCertManager),
		addVolume:        vmrest.NewAddVolumeREST(vmLister, kubevirt, proxyCertManager),
		removeVolume:     vmrest.NewRemoveVolumeREST(vmLister, kubevirt, proxyCertManager),
		freeze:           vmrest.NewFreezeREST(vmLister, kubevirt, proxyCertManager),
		unfreeze:         vmrest.NewUnfreezeREST(vmLister, kubevirt, proxyCertManager),
		cancelEvacuation: vmrest.NewCancelEvacuationREST(vmLister, kubevirt, proxyCertManager),
		convertor:        convertor,
		vmClient:         vmClient,
	}
}

func (store LegacyVirtualMachineStorage) ConsoleREST() *vmrest.ConsoleREST {
	return store.console
}

func (store LegacyVirtualMachineStorage) VncREST() *vmrest.VNCREST {
	return store.vnc
}

func (store LegacyVirtualMachineStorage) PortForwardREST() *vmrest.PortForwardREST {
	return store.portforward
}

func (store LegacyVirtualMachineStorage) AddVolumeREST() *vmrest.AddVolumeREST {
	return store.addVolume
}

func (store LegacyVirtualMachineStorage) RemoveVolumeREST() *vmrest.RemoveVolumeREST {
	return store.removeVolume
}

func (store LegacyVirtualMachineStorage) FreezeREST() *vmrest.FreezeREST {
	return store.freeze
}

func (store LegacyVirtualMachineStorage) UnfreezeREST() *vmrest.UnfreezeREST {
	return store.unfreeze
}

func (store LegacyVirtualMachineStorage) CancelEvacuationREST() *vmrest.CancelEvacuationREST {
	return store.cancelEvacuation
}

// New implements rest.Storage interface
func (store LegacyVirtualMachineStorage) New() runtime.Object {
	return &v1alpha2.VirtualMachine{}
}

// Destroy implements rest.Storage interface
func (store LegacyVirtualMachineStorage) Destroy() {
}

// Kind implements rest.KindProvider interface
func (store LegacyVirtualMachineStorage) Kind() string {
	return "VirtualMachine"
}

// NamespaceScoped implements rest.Scoper interface
func (store LegacyVirtualMachineStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName implements rest.SingularNameProvider interface
func (store LegacyVirtualMachineStorage) GetSingularName() string {
	return "virtualmachine"
}

func (store LegacyVirtualMachineStorage) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)
	vm, err := store.vmLister.VirtualMachines(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, k8serrors.NewNotFound(subresources.Resource("virtualmachines"), name)
		}
		return nil, k8serrors.NewInternalError(err)
	}
	return vm, nil
}

func (store LegacyVirtualMachineStorage) NewList() runtime.Object {
	return &v1alpha2.VirtualMachineList{}
}

func (store LegacyVirtualMachineStorage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
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

	filtered := &v1alpha2.VirtualMachineList{}
	filtered.Items = make([]v1alpha2.VirtualMachine, 0, len(items))
	for _, vm := range items {
		if matches(vm, name) {
			filtered.Items = append(filtered.Items, *vm)
		}
	}
	return filtered, nil
}

func (store LegacyVirtualMachineStorage) ConvertToTable(ctx context.Context, object, tableOptions runtime.Object) (*metav1.Table, error) {
	if store.convertor != nil {
		return store.convertor.ConvertToTable(ctx, object, tableOptions)
	}
	return rest.NewDefaultTableConvertor(subresources.Resource("virtualmachines")).ConvertToTable(ctx, object, tableOptions)
}

func (store LegacyVirtualMachineStorage) Delete(ctx context.Context, name string, _ rest.ValidateObjectFunc, options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	var opts metav1.DeleteOptions
	if options != nil {
		opts = *options
	}
	if err := store.vmClient.VirtualMachines(genericreq.NamespaceValue(ctx)).Delete(ctx, name, opts); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, false, k8serrors.NewNotFound(subresources.Resource("virtualmachines"), name)
		}
		return nil, false, k8serrors.NewInternalError(err)
	}
	return nil, true, nil
}

func nameFor(fs fields.Selector) (string, error) {
	if fs == nil || fs.Empty() {
		fs = fields.Everything()
	}
	name, found := fs.RequiresExactMatch("metadata.name")
	if !found && !fs.Empty() {
		return "", fmt.Errorf("field label not supported: %s", fs.Requirements()[0].Field)
	}
	return name, nil
}

func matches(obj metav1.Object, name string) bool {
	if name == "" {
		name = obj.GetName()
	}
	return obj.GetName() == name
}
