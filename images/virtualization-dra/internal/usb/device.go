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
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/deckhouse/virtualization-dra/pkg/usb"
)

type DeviceSet = sets.Set[Device]

func NewDeviceSet() DeviceSet {
	return sets.New[Device]()
}

type Device struct {
	Path         string
	BusID        string
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

func (d *Device) GetName(nodeName string) string {
	// usb-<bus>-<deviceNumber>-<vendorID>-<productID>-<nodeName>
	// usb-003-005-e39-f100
	return fmt.Sprintf("usb-%s-%s-%s-%s-%s", d.Bus.String(), d.DeviceNumber.String(), d.VendorID.String(), d.ProductID.String(), nodeName)
}

func (d *Device) Validate() error {
	if d.BusID == "" {
		return fmt.Errorf("BusID is required")
	}
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

func toDevice(device *usb.Device) Device {
	return Device{
		Path:         device.Path,
		BusID:        device.BusID,
		Manufacturer: device.Manufacturer,
		Product:      device.Product,
		VendorID:     int4x(device.VendorID),
		ProductID:    int4x(device.ProductID),
		BCD:          int4x(device.BCD),
		Bus:          int3d(device.Bus),
		DeviceNumber: int3d(device.DeviceNumber),
		Major:        int(device.Major),
		Minor:        int(device.Minor),
		Serial:       device.Serial,
		DevicePath:   device.DevicePath,
	}
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
