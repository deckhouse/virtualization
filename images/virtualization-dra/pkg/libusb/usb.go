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

package libusb

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const PathToUSBDevices = "/sys/bus/usb/devices"

var pathToUSBDevices = PathToUSBDevices

type USBDevice struct {
	Path                string               `json:"path"`
	BusID               string               `json:"busID"`
	Manufacturer        string               `json:"manufacturer"`
	Product             string               `json:"product"`
	Serial              string               `json:"serial"`
	DevicePath          string               `json:"devicePath"`
	Driver              string               `json:"driver"`
	IsHub               bool                 `json:"isHub"`
	VendorID            uint16               `json:"vendorID"`
	ProductID           uint16               `json:"productID"`
	BCD                 uint16               `json:"bcd"`
	Bus                 uint32               `json:"bus"`
	DeviceNumber        uint32               `json:"deviceNumber"`
	Speed               uint32               `json:"speed"`
	Major               uint64               `json:"major"`
	Minor               uint64               `json:"minor"`
	BDeviceClass        uint8                `json:"bDeviceClass"`
	BDeviceSubClass     uint8                `json:"bDeviceSubClass"`
	BDeviceProtocol     uint8                `json:"bDeviceProtocol"`
	BConfigurationValue uint8                `json:"bConfigurationValue"`
	BNumConfigurations  uint8                `json:"bNumConfigurations"`
	BNumInterfaces      uint8                `json:"bNumInterfaces"`
	Interfaces          []USBDeviceInterface `json:"interfaces"`
}

type USBDeviceInterface struct {
	BInterfaceClass    uint8 `json:"bInterfaceClass"`
	BInterfaceSubClass uint8 `json:"bInterfaceSubClass"`
	BInterfaceProtocol uint8 `json:"bInterfaceProtocol"`
}

func (d *USBDevice) Equal(other *USBDevice) bool {
	return d.Path == other.Path &&
		d.BusID == other.BusID &&
		d.Manufacturer == other.Manufacturer &&
		d.Product == other.Product &&
		d.Serial == other.Serial &&
		d.DevicePath == other.DevicePath &&
		d.Driver == other.Driver &&
		d.IsHub == other.IsHub &&
		d.VendorID == other.VendorID &&
		d.ProductID == other.ProductID &&
		d.BCD == other.BCD &&
		d.Bus == other.Bus &&
		d.DeviceNumber == other.DeviceNumber &&
		d.Major == other.Major &&
		d.Minor == other.Minor &&
		d.BDeviceClass == other.BDeviceClass &&
		d.BDeviceSubClass == other.BDeviceSubClass &&
		d.BDeviceProtocol == other.BDeviceProtocol &&
		d.BConfigurationValue == other.BConfigurationValue &&
		d.BNumConfigurations == other.BNumConfigurations &&
		d.BNumInterfaces == other.BNumInterfaces &&
		slices.Equal(d.Interfaces, other.Interfaces)
}

func (d *USBDevice) Validate() error {
	if d.VendorID == 0 {
		return fmt.Errorf("vendorID is required")
	}
	if d.ProductID == 0 {
		return fmt.Errorf("productID is required")
	}
	if d.Bus == 0 {
		return fmt.Errorf("bus is required")
	}
	if d.DeviceNumber == 0 {
		return fmt.Errorf("deviceNumber is required")
	}
	if d.DevicePath == "" {
		return fmt.Errorf("devicePath is required")
	}
	if d.Major == 0 {
		return fmt.Errorf("major is required")
	}
	if d.Minor == 0 {
		return fmt.Errorf("minor is required")
	}
	return nil
}

func LoadUSBDevice(path string) (device USBDevice, err error) {
	if !strings.HasPrefix(path, pathToUSBDevices) {
		return device, fmt.Errorf("path %s is not a usb device", path)
	}

	device.Path = path
	device.BusID = filepath.Base(path)

	if err = parseSysUeventFile(path, &device); err != nil {
		return device, err
	}
	if err = parseSerial(path, &device); err != nil {
		return device, err
	}
	if err = parseManufacturer(path, &device); err != nil {
		return device, err
	}
	if err = parseProduct(path, &device); err != nil {
		return device, err
	}
	if err = parseBConfigurationValue(path, &device); err != nil {
		return device, err
	}
	if err = parseBNumConfigurations(path, &device); err != nil {
		return device, err
	}
	if err = parseBNumInterfaces(path, &device); err != nil {
		return device, err
	}
	if err = parseSpeed(path, &device); err != nil {
		return device, err
	}
	if err = parseSysUeventInterfaces(path, &device); err != nil {
		return device, err
	}

	return device, err
}

func parseSysUeventFile(path string, device *USBDevice) error {
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
	ueventPath := filepath.Join(path, "uevent")
	file, err := os.Open(ueventPath)
	if err != nil {
		return fmt.Errorf("unable to open the file %s: %w", ueventPath, err)
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
			val, err := strconv.ParseUint(values[1], 10, 32)
			if err != nil {
				return fmt.Errorf("failed to parse MAJOR: %s", values[1])
			}
			device.Major = val
		case "MINOR":
			val, err := strconv.ParseUint(values[1], 10, 32)
			if err != nil {
				return fmt.Errorf("failed to parse MINOR: %s", values[1])
			}
			device.Minor = val
		case "DEVNAME":
			device.DevicePath = filepath.Join("/dev", values[1])
		case "DRIVER":
			device.Driver = values[1]
		case "PRODUCT":
			products := strings.Split(values[1], "/")
			if len(products) != 3 {
				slog.Error("Failed to parse PRODUCT", slog.String("value", values[1]), slog.Any("err", err))
				return fmt.Errorf("failed to parse PRODUCT: %s", values[1])
			}

			val, err := strconv.ParseUint(products[0], 16, 32)
			if err != nil {
				return fmt.Errorf("failed to parse PRODUCT: %s", values[1])
			}
			device.VendorID = uint16(val)

			val, err = strconv.ParseUint(products[1], 16, 32)
			if err != nil {
				return fmt.Errorf("failed to parse PRODUCT: %s", values[1])
			}
			device.ProductID = uint16(val)

			val, err = strconv.ParseUint(products[2], 16, 32)
			if err != nil {
				return fmt.Errorf("failed to parse PRODUCT: %s", values[1])
			}
			device.BCD = uint16(val)
		case "TYPE":
			types := strings.Split(values[1], "/")
			if len(types) != 3 {
				return fmt.Errorf("failed to parse TYPE: %s", values[1])
			}
			val, err := strconv.ParseUint(types[0], 10, 8)
			if err != nil {
				return fmt.Errorf("failed to parse TYPE: %s", values[1])
			}
			device.BDeviceClass = uint8(val)
			device.IsHub = device.BDeviceClass == 9 // 09 = USB Hub class

			val, err = strconv.ParseUint(types[1], 10, 8)
			if err != nil {
				return fmt.Errorf("failed to parse TYPE: %s", values[1])
			}
			device.BDeviceSubClass = uint8(val)

			val, err = strconv.ParseUint(types[2], 10, 8)
			if err != nil {
				return fmt.Errorf("failed to parse TYPE: %s", values[1])
			}
			device.BDeviceProtocol = uint8(val)
		case "BUSNUM":
			val, err := strconv.ParseUint(values[1], 10, 32)
			if err != nil {
				return fmt.Errorf("failed to parse BUSNUM: %s", values[1])
			}
			device.Bus = uint32(val)
		case "DEVNUM":
			val, err := strconv.ParseUint(values[1], 10, 32)
			if err != nil {
				return fmt.Errorf("failed to parse DEVNUM: %s", values[1])
			}
			device.DeviceNumber = uint32(val)
		default:
		}
	}
	return nil
}

func parseSerial(path string, device *USBDevice) error {
	serial, err := parseStringValue(path, "serial")
	if err != nil {
		return err
	}
	device.Serial = serial
	return nil
}

func parseManufacturer(path string, device *USBDevice) error {
	manufacturer, err := parseStringValue(path, "manufacturer")
	if err != nil {
		return err
	}
	device.Manufacturer = manufacturer
	return nil
}

func parseProduct(path string, device *USBDevice) error {
	product, err := parseStringValue(path, "product")
	if err != nil {
		return err
	}
	device.Product = product
	return nil
}

func parseBConfigurationValue(path string, device *USBDevice) error {
	val, err := parseUintValue(path, "bConfigurationValue", 8, true)
	if err != nil {
		return err
	}
	device.BConfigurationValue = uint8(val)
	return nil
}

func parseBNumConfigurations(path string, device *USBDevice) error {
	val, err := parseUintValue(path, "bNumConfigurations", 8, false)
	if err != nil {
		return err
	}
	device.BNumConfigurations = uint8(val)
	return nil
}

func parseBNumInterfaces(path string, device *USBDevice) error {
	val, err := parseUintValue(path, "bNumInterfaces", 8, true)
	if err != nil {
		return err
	}
	device.BNumInterfaces = uint8(val)
	return nil
}

func parseSpeed(path string, device *USBDevice) error {
	val, err := parseUintValue(path, "speed", 32, false)
	if err != nil {
		return err
	}
	device.Speed = uint32(val)
	return nil
}

func parseSysUeventInterfaces(path string, device *USBDevice) error {
	// 3-2.1.1:1.0
	// |       | |
	// │       | |- bInterfaceNumber
	// |       |--- bConfigurationValue
	// |----------- busID usb_device

	// path - /sys/bus/usb/devices/3-2.1.1
	// uevent path - /sys/bus/usb/devices/3-2.1.1:1.0/uevent

	if device.BConfigurationValue == 0 || device.BNumInterfaces == 0 {
		device.Interfaces = nil
		return nil
	}

	var deviceInterfaces []USBDeviceInterface

	parent := filepath.Dir(path)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return fmt.Errorf("unable to read the directory %s: %w", path, err)
	}

	prefix := fmt.Sprintf("%s:%d.", device.BusID, device.BConfigurationValue)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		interfaceNumberStr := strings.TrimPrefix(entry.Name(), prefix)
		_, err := strconv.Atoi(interfaceNumberStr)
		if err != nil {
			// not a valid interface number
			continue
		}

		ueventPath := filepath.Join(parent, entry.Name(), "uevent")
		file, err := os.Open(ueventPath)
		if err != nil {
			return fmt.Errorf("unable to open the file %s: %w", ueventPath, err)
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
			if values[0] == "INTERFACE" {
				deviceInterface := USBDeviceInterface{}

				interfaces := strings.Split(values[1], "/")
				if len(interfaces) != 3 {
					return fmt.Errorf("failed to parse INTERFACE: %s", values[1])
				}
				val, err := strconv.ParseUint(interfaces[0], 10, 8)
				if err != nil {
					return fmt.Errorf("failed to parse INTERFACE: %s", values[1])
				}
				deviceInterface.BInterfaceClass = uint8(val)

				val, err = strconv.ParseUint(interfaces[1], 10, 8)
				if err != nil {
					return fmt.Errorf("failed to parse INTERFACE: %s", values[1])
				}
				deviceInterface.BInterfaceSubClass = uint8(val)

				val, err = strconv.ParseUint(interfaces[2], 10, 8)
				if err != nil {
					return fmt.Errorf("failed to parse INTERFACE: %s", values[1])
				}
				deviceInterface.BInterfaceProtocol = uint8(val)

				deviceInterfaces = append(deviceInterfaces, deviceInterface)

				break
			}
		}
	}

	device.Interfaces = deviceInterfaces

	return nil
}

func parseStringValue(path, valueName string) (string, error) {
	valuePath := filepath.Join(path, valueName)
	data, err := os.ReadFile(valuePath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", valuePath, err)
	}

	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("invalid %s: empty", valueName)
	}

	return value, nil
}

func parseUintValue(path, valueName string, bitSize int, ignoreNotExist bool) (uint64, error) {
	valuePath := filepath.Join(path, valueName)
	data, err := os.ReadFile(valuePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", path, err)
	}

	value := strings.TrimSpace(string(data))
	if value == "" && ignoreNotExist {
		return 0, nil
	}

	val, err := strconv.ParseUint(value, 10, bitSize)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s: %w", valueName, err)
	}

	if val == 0 {
		return 0, fmt.Errorf("invalid %s: 0", valueName)
	}

	return val, nil
}
