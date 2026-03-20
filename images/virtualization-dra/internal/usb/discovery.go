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
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/deckhouse/virtualization-dra/internal/featuregates"
)

func (s *AllocationStore) discoveryPluggedUSBDevices(ctx context.Context) (map[string]Device, DeviceSet, error) {
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

	usbDeviceMap := make(map[string]Device)
	usbipDeviceSet := NewDeviceSet()

	for _, device := range allUSBDevices {
		if _, ok := busIDSet[device.BusID]; ok {
			usbipDeviceSet.Insert(toDevice(&device))
		} else {
			// usb device can be not exists in attach info because usbip detached it
			// while it is still present in sysfs because vhci_hcd has not fully released it yet.
			isUSBIPDevice, err := isUSBIPDevice(device.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to check if usb device is usbip device: %w", err)
			}
			if isUSBIPDevice {
				continue
			}

			dev := toDevice(&device)
			usbDeviceMap[dev.GetName(s.nodeName)] = dev
		}
	}

	if s.log.Enabled(ctx, slog.LevelDebug) {
		s.log.Debug("USB device set", slog.Any("usbDeviceMap", usbDeviceMap))
		s.log.Debug("USBIP device set", slog.Any("usbipDeviceSet", usbipDeviceSet.UnsortedList()))
	}

	return usbDeviceMap, usbipDeviceSet, nil
}

func isUSBIPDevice(devPath string) (bool, error) {
	realPath, err := filepath.EvalSymlinks(devPath)
	if err != nil {
		return false, err
	}

	realPath = filepath.Clean(realPath)

	return strings.Contains(realPath, "vhci_hcd"), nil
}
