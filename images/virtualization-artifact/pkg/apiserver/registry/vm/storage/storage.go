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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	versionedv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type VirtualMachineStorage struct {
	vmLister         virtlisters.VirtualMachineLister
	console          *vmrest.ConsoleREST
	vnc              *vmrest.VNCREST
	portforward      *vmrest.PortForwardREST
	addVolume        *vmrest.AddVolumeREST
	removeVolume     *vmrest.RemoveVolumeREST
	freeze           *vmrest.FreezeREST
	unfreeze         *vmrest.UnfreezeREST
	cancelEvacuation *vmrest.CancelEvacuationREST
	usbRedir         *vmrest.USBRedirREST
	vmClient         versionedv1alpha2.VirtualMachinesGetter
}

var (
	_ rest.KindProvider         = &VirtualMachineStorage{}
	_ rest.Storage              = &VirtualMachineStorage{}
	_ rest.Scoper               = &VirtualMachineStorage{}
	_ rest.Getter               = &VirtualMachineStorage{}
	_ rest.SingularNameProvider = &VirtualMachineStorage{}
)

func NewStorage(
	vmLister virtlisters.VirtualMachineLister,
	kubevirt vmrest.KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	vmClient versionedv1alpha2.VirtualMachinesGetter,
) *VirtualMachineStorage {
	return &VirtualMachineStorage{
		vmLister:         vmLister,
		console:          vmrest.NewConsoleREST(vmLister, kubevirt, proxyCertManager),
		vnc:              vmrest.NewVNCREST(vmLister, kubevirt, proxyCertManager),
		portforward:      vmrest.NewPortForwardREST(vmLister, kubevirt, proxyCertManager),
		addVolume:        vmrest.NewAddVolumeREST(vmLister, kubevirt, proxyCertManager),
		removeVolume:     vmrest.NewRemoveVolumeREST(vmLister, kubevirt, proxyCertManager),
		freeze:           vmrest.NewFreezeREST(vmLister, kubevirt, proxyCertManager),
		unfreeze:         vmrest.NewUnfreezeREST(vmLister, kubevirt, proxyCertManager),
		cancelEvacuation: vmrest.NewCancelEvacuationREST(vmLister, kubevirt, proxyCertManager),
		usbRedir:         vmrest.NewUSBRedirREST(vmLister, kubevirt, proxyCertManager),
		vmClient:         vmClient,
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

func (store VirtualMachineStorage) CancelEvacuationREST() *vmrest.CancelEvacuationREST {
	return store.cancelEvacuation
}

func (store VirtualMachineStorage) USBRedirREST() *vmrest.USBRedirREST {
	return store.usbRedir
}

// New implements rest.Storage interface
func (store VirtualMachineStorage) New() runtime.Object {
	return &subv1alpha2.VirtualMachine{}
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
			return nil, k8serrors.NewNotFound(subresources.Resource("virtualmachines"), name)
		}
		return nil, k8serrors.NewInternalError(err)
	}
	return &subresources.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: subresources.SchemeGroupVersion.String(),
			Kind:       "VirtualMachine",
		},
		ObjectMeta: vm.ObjectMeta,
	}, nil
}
