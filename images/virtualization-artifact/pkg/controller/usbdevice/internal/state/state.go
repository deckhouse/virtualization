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

package state

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type USBDeviceState interface {
	USBDevice() *reconciler.Resource[*v1alpha2.USBDevice, v1alpha2.USBDeviceStatus]
	NodeUSBDevice(ctx context.Context) (*v1alpha2.NodeUSBDevice, error)
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
	// We need to find the NodeUSBDevice by name across all namespaces
	usbDevice := s.usbDevice.Current()
	if usbDevice == nil {
		return nil, nil
	}

	var nodeUSBDeviceList v1alpha2.NodeUSBDeviceList
	if err := s.client.List(ctx, &nodeUSBDeviceList); err != nil {
		return nil, err
	}

	// Find the NodeUSBDevice that matches by name
	for i := range nodeUSBDeviceList.Items {
		if nodeUSBDeviceList.Items[i].Name == usbDevice.Name {
			return &nodeUSBDeviceList.Items[i], nil
		}
	}

	return nil, nil
}
