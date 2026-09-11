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

package state

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PCIDeviceState interface {
	PCIDevice() *reconciler.Resource[*v1alpha2.PCIDevice, v1alpha2.PCIDeviceStatus]
	NodePCIDevice(ctx context.Context) (*v1alpha2.NodePCIDevice, error)
	VirtualMachinesReferencingDevice(ctx context.Context) ([]*v1alpha2.VirtualMachine, error)
}

func New(client client.Client, pciDevice *reconciler.Resource[*v1alpha2.PCIDevice, v1alpha2.PCIDeviceStatus]) PCIDeviceState {
	return &pciDeviceState{
		client:    client,
		pciDevice: pciDevice,
	}
}

type pciDeviceState struct {
	client    client.Client
	pciDevice *reconciler.Resource[*v1alpha2.PCIDevice, v1alpha2.PCIDeviceStatus]
}

func (s *pciDeviceState) PCIDevice() *reconciler.Resource[*v1alpha2.PCIDevice, v1alpha2.PCIDeviceStatus] {
	return s.pciDevice
}

func (s *pciDeviceState) NodePCIDevice(ctx context.Context) (*v1alpha2.NodePCIDevice, error) {
	pciDevice := s.pciDevice.Current()
	if pciDevice == nil {
		return nil, nil
	}

	nodePCIDevice := &v1alpha2.NodePCIDevice{}
	err := s.client.Get(ctx, client.ObjectKey{Name: pciDevice.Name}, nodePCIDevice)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return nodePCIDevice, nil
}

func (s *pciDeviceState) VirtualMachinesReferencingDevice(ctx context.Context) ([]*v1alpha2.VirtualMachine, error) {
	pciDevice := s.pciDevice.Current()
	if pciDevice == nil {
		return nil, nil
	}

	var vmList v1alpha2.VirtualMachineList
	if err := s.client.List(ctx, &vmList, client.MatchingFields{
		indexer.IndexFieldVMByPCIDevice: pciDevice.Name,
	}); err != nil {
		return nil, err
	}

	var result []*v1alpha2.VirtualMachine
	for i := range vmList.Items {
		vm := &vmList.Items[i]
		if vm.Namespace == pciDevice.Namespace {
			result = append(result, vm)
		}
	}

	return result, nil
}
