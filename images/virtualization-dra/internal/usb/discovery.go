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

package usb

import (
	"github.com/deckhouse/virtualization-dra/internal/featuregates"
	"github.com/deckhouse/virtualization-dra/internal/usbip"
	"github.com/deckhouse/virtualization-dra/pkg/usb"
)

const PathToUSBDevices = usb.PathToUSBDevices

func newDiscoverer() discoverer {
	return discoverer{
		getter: usbip.NewUSBAttacher(),
	}
}

type discoverer struct {
	getter usbip.AttachInfoGetter
}

func (d *discoverer) DiscoveryPluggedUSBDevices(pathToUSBDevices string) (DeviceSet, DeviceSet, error) {
	devices, err := usb.DiscoverPluggedUSBDevices(pathToUSBDevices)
	if err != nil {
		return nil, nil, err
	}

	busIdMaps := make(map[string]struct{})
	if featuregates.Default().USBGatewayEnabled() {
		infos, err := d.getter.GetAttachInfo()
		if err != nil {
			return nil, nil, err
		}
		for _, info := range infos {
			busIdMaps[info.LocalBusID] = struct{}{}
		}
	}

	usbDeviceSet := NewDeviceSet()
	usbipDeviceSet := NewDeviceSet()

	for _, device := range devices {
		if _, ok := busIdMaps[device.BusID]; ok {
			usbipDeviceSet.Insert(toDevice(device))
		} else {
			usbDeviceSet.Insert(toDevice(device))
		}
	}

	return usbDeviceSet, usbipDeviceSet, nil
}
