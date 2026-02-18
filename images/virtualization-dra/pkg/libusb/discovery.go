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

package libusb

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func DiscoverPluggedUSBDevices() (map[string]*USBDevice, error) {
	devices := make(map[string]*USBDevice)

	entries, err := os.ReadDir(pathToUSBDevices)
	if err != nil {
		return nil, fmt.Errorf("failed to read usb devices directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(pathToUSBDevices, entry.Name())

		if !isUsbPath(path) {
			continue
		}

		// Get device information
		device, err := LoadUSBDevice(path)
		if err != nil {
			return nil, err
		}

		if err = device.Validate(); err != nil {
			slog.Error("failed to validate device, skip...", slog.Any("device", device), slog.String("error", err.Error()))
			continue
		}

		devices[path] = &device
	}

	return devices, nil
}

func isUsbPath(path string) bool {
	// Ignore named usb controllers
	if strings.HasPrefix(filepath.Base(path), "usb") {
		return false
	}
	// We are interested in actual USB devices information that
	// contains idVendor and idProduct. We can skip all others.
	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(path, file)); err != nil {
			return false
		}
	}

	return true
}

var requiredFiles = []string{
	"idVendor",
	"idProduct",
	"uevent",
	"serial",
	"manufacturer",
	"product",
	"bConfigurationValue",
	"bNumConfigurations",
	"bNumInterfaces",
	"speed",
}
