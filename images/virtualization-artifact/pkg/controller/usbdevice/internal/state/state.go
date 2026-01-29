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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type USBDeviceState interface {
	USBDevice() *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus]
	NodeUSBDevice(ctx context.Context) (*v1alpha2.NodeUSBDevice, error)
	VirtualMachinesUsingDevice(ctx context.Context) ([]*v1alpha2.VirtualMachine, error)
}

func New(client client.Client, usbDevice *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus]) USBDeviceState {
	return &usbDeviceState{
		client:    client,
		usbDevice: usbDevice,
	}
}

type usbDeviceState struct {
	client    client.Client
	usbDevice *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus]
}

func (s *usbDeviceState) USBDevice() *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus] {
	return s.usbDevice
}

func (s *usbDeviceState) NodeUSBDevice(ctx context.Context) (*v1alpha2.NodeUSBDevice, error) {
	// USBDevice has the same name as the corresponding NodeUSBDevice
	// Use indexer to find NodeUSBDevice by name
	usbDevice := s.usbDevice.Current()
	if usbDevice == nil {
		return nil, nil
	}

	var nodeUSBDeviceList v1alpha2.NodeUSBDeviceList
	if err := s.client.List(ctx, &nodeUSBDeviceList, client.MatchingFields{
		indexer.IndexFieldNodeUSBDeviceByName: usbDevice.Name,
	}); err != nil {
		return nil, err
	}

	if len(nodeUSBDeviceList.Items) > 0 {
		return &nodeUSBDeviceList.Items[0], nil
	}

	return nil, nil
}

func (s *usbDeviceState) VirtualMachinesUsingDevice(ctx context.Context) ([]*v1alpha2.VirtualMachine, error) {
	usbDevice := s.usbDevice.Current()
	if usbDevice == nil {
		return nil, nil
	}

	var vmList v1alpha2.VirtualMachineList
	if err := s.client.List(ctx, &vmList, client.MatchingFields{
		indexer.IndexFieldVMByUSBDevice: usbDevice.Name,
	}); err != nil {
		return nil, err
	}

	var result []*v1alpha2.VirtualMachine
	for i := range vmList.Items {
		vm := &vmList.Items[i]
		// Check if VM is in the same namespace as USBDevice
		if vm.Namespace == usbDevice.Namespace {
			// Verify that device is actually attached in VM status
			for _, usbStatus := range vm.Status.USBDevices {
				if usbStatus.Name == usbDevice.Name && usbStatus.Attached {
					result = append(result, vm)
					break
				}
			}
		}
	}

	return result, nil
}
