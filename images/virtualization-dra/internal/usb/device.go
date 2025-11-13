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
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/deckhouse/virtualization-dra/pkg/set"
)

type DeviceSet = set.Set[Device]

func NewDeviceSet() *DeviceSet {
	return set.New[Device]()
}

type Device struct {
	Name         string
	Manufacturer string
	Product      string
	VendorID     int4x
	ProductID    int4x
	BCD          int4x
	Bus          int3d
	DeviceNumber int3d
	Major        int
	Minor        int
	Serial       string
	DevicePath   string
}

func (d *Device) GetName() string {
	// usb-<bus>-<deviceNumber>-<vendorID>-<productID>
	// usb-003-005-e39-f100
	return fmt.Sprintf("usb-%s-%s-%s-%s", d.Bus.String(), d.DeviceNumber.String(), d.VendorID.String(), d.ProductID.String())
}

func (d *Device) Validate() error {
	if d.VendorID == 0 {
		return fmt.Errorf("VendorID is required")
	}
	if d.ProductID == 0 {
		return fmt.Errorf("ProductID is required")
	}
	if d.Bus == 0 {
		return fmt.Errorf("Bus is required")
	}
	if d.DeviceNumber == 0 {
		return fmt.Errorf("DeviceNumber is required")
	}
	if d.DevicePath == "" {
		return fmt.Errorf("DevicePath is required")
	}
	if d.Major == 0 {
		return fmt.Errorf("Major is required")
	}
	if d.Minor == 0 {
		return fmt.Errorf("Minor is required")
	}
	return nil
}

func LoadDevice(path string) (device Device, err error) {
	if err = parseSysUeventFile(path, &device); err != nil {
		return
	}
	if err = parseSerial(path, &device); err != nil {
		return
	}
	if err = parseManufacturer(path, &device); err != nil {
		return
	}
	if err = parseProduct(path, &device); err != nil {
		return
	}
	return
}

func parseSysUeventFile(path string, device *Device) error {
	// Example uevent file:
	// MAJOR=189
	// MINOR=257
	// DEVNAME=bus/usb/003/002
	// DEVTYPE=usb_device
	// DRIVER=usb
	// PRODUCT=e39/f100/35d
	// TYPE=0/0/0
	// BUSNUM=003
	// DEVNUM=002
	file, err := os.Open(filepath.Join(path, "uevent"))
	if err != nil {
		return fmt.Errorf("unable to open the file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		values := strings.Split(line, "=")
		if len(values) != 2 {
			slog.Info("Skipping %s due not being key=value", slog.String("line", line))
			continue
		}
		switch values[0] {
		case "MAJOR":
			val, err := strconv.ParseInt(values[1], 10, 32)
			if err != nil {
				slog.Error("Failed to parse MAJOR", slog.String("value", values[1]), slog.Any("err", err))
				return nil
			}
			device.Major = int(val)
		case "MINOR":
			val, err := strconv.ParseInt(values[1], 10, 32)
			if err != nil {
				slog.Error("Failed to parse MINOR", slog.String("value", values[1]), slog.Any("err", err))
				return nil
			}
			device.Minor = int(val)
		case "BUSNUM":
			val, err := strconv.ParseInt(values[1], 10, 32)
			if err != nil {
				slog.Error("Failed to parse BUSNUM", slog.String("value", values[1]), slog.Any("err", err))
				return nil
			}
			device.Bus = int3d(val)
		case "DEVNUM":
			val, err := strconv.ParseInt(values[1], 10, 32)
			if err != nil {
				slog.Error("Failed to parse DEVNUM", slog.String("value", values[1]), slog.Any("err", err))
				return nil
			}
			device.DeviceNumber = int3d(val)
		case "PRODUCT":
			products := strings.Split(values[1], "/")
			if len(products) != 3 {
				slog.Error("Failed to parse PRODUCT", slog.String("value", values[1]), slog.Any("err", err))
				return nil
			}

			val, err := strconv.ParseInt(products[0], 16, 32)
			if err != nil {
				slog.Error("Failed to parse PRODUCT", slog.String("value", values[1]), slog.Any("err", err))
				return nil
			}
			device.VendorID = int4x(val)

			val, err = strconv.ParseInt(products[1], 16, 32)
			if err != nil {
				slog.Error("Failed to parse PRODUCT", slog.String("value", values[1]), slog.Any("err", err))
				return nil
			}
			device.ProductID = int4x(val)

			val, err = strconv.ParseInt(products[2], 16, 32)
			if err != nil {
				slog.Error("Failed to parse PRODUCT", slog.String("value", values[1]), slog.Any("err", err))
				return nil
			}
			device.BCD = int4x(val)
		case "DEVNAME":
			device.DevicePath = filepath.Join("/dev", values[1])
		default:
			slog.Info("Skipping unhandled line", slog.String("line", line))
		}
	}
	return nil
}

func parseSerial(path string, device *Device) error {
	b, err := os.ReadFile(filepath.Join(path, "serial"))
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) >= 1 {
		device.Serial = strings.TrimSpace(lines[0])
	} else {
		device.Serial = "unknown"
	}

	return nil
}

func parseManufacturer(path string, device *Device) error {
	b, err := os.ReadFile(filepath.Join(path, "manufacturer"))
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) >= 1 {
		device.Manufacturer = strings.TrimSpace(lines[0])
	} else {
		device.Manufacturer = "unknown"
	}
	return nil
}

func parseProduct(path string, device *Device) error {
	b, err := os.ReadFile(filepath.Join(path, "product"))
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) >= 1 {
		device.Product = strings.TrimSpace(lines[0])
	} else {
		device.Product = "unknown"
	}
	return nil
}

type int4x int

func (i int4x) String() string {
	s := strconv.FormatInt(int64(i), 16)
	if len(s) < 4 {
		return strings.Repeat("0", 4-len(s)) + s
	}
	return s
}

type int3d int

func (i int3d) String() string {
	s := strconv.FormatInt(int64(i), 10)
	if len(s) < 3 {
		return strings.Repeat("0", 3-len(s)) + s
	}
	return s
}
