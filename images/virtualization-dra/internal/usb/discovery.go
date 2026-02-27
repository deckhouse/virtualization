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
	"log/slog"

	"github.com/deckhouse/virtualization-dra/internal/featuregates"
)

func (s *AllocationStore) discoveryPluggedUSBDevices() (DeviceSet, DeviceSet, error) {
	allUSBDevices := s.monitor.GetDevices()

	busIDSet := make(map[string]struct{})
	if featuregates.Default().USBGatewayEnabled() {
		info, err := s.usbipInfoGetter.GetAttachInfo()
		if err != nil {
			return nil, nil, err
		}
		for _, item := range info.Items {
			busIDSet[item.LocalBusID] = struct{}{}
		}
	}

	usbDeviceSet := NewDeviceSet()
	usbipDeviceSet := NewDeviceSet()

	for _, device := range allUSBDevices {
		if _, ok := busIDSet[device.BusID]; ok {
			usbipDeviceSet.Insert(toDevice(&device))
		} else {
			usbDeviceSet.Insert(toDevice(&device))
		}
	}

	s.log.Debug("USB device set", slog.Any("usbDeviceSet", usbDeviceSet))
	s.log.Debug("USBIP device set", slog.Any("usbipDeviceSet", usbipDeviceSet))

	return usbDeviceSet, usbipDeviceSet, nil
}
