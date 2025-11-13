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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const PathToUSBDevices = "/sys/bus/usb/devices"

func discoverPluggedUSBDevices(pathToUSBDevices string) (*DeviceSet, error) {
	usbDeviceSet := NewDeviceSet()
	err := filepath.Walk(pathToUSBDevices, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Ignore named usb controllers
		if strings.HasPrefix(info.Name(), "usb") {
			return nil
		}
		// We are interested in actual USB devices information that
		// contains idVendor and idProduct. We can skip all others.
		if _, err := os.Stat(filepath.Join(path, "idVendor")); err != nil {
			return nil
		}

		// Get device information
		device, err := LoadDevice(path)
		if err = device.Validate(); err != nil {
			slog.Error("failed to validate device, skip...", slog.Any("device", device), slog.String("error", err.Error()))
			return nil
		}
		if err != nil {
			return err
		}
		usbDeviceSet.Add(device)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed when walking usb devices tree: %w", err)
	}
	return usbDeviceSet, nil
}
