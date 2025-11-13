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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUSB(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "USB Suite")
}

func compareUsb(a, b *USBDevice) {
	Expect(a.Path).To(Equal(b.Path), "Path")
	Expect(a.BusID).To(Equal(b.BusID), "BusID")
	Expect(a.Manufacturer).To(Equal(b.Manufacturer), "Manufacturer")
	Expect(a.Product).To(Equal(b.Product), "Product")
	Expect(a.Serial).To(Equal(b.Serial), "Serial")
	Expect(a.DevicePath).To(Equal(b.DevicePath), "DevicePath")
	Expect(a.Driver).To(Equal(b.Driver), "Driver")
	Expect(a.IsHub).To(Equal(b.IsHub), "IsHub")
	Expect(a.VendorID).To(Equal(b.VendorID), "VendorID")
	Expect(a.ProductID).To(Equal(b.ProductID), "ProductID")
	Expect(a.BCD).To(Equal(b.BCD), "BCD")
	Expect(a.Bus).To(Equal(b.Bus), "Bus")
	Expect(a.DeviceNumber).To(Equal(b.DeviceNumber), "DeviceNumber")
	Expect(a.Speed).To(Equal(b.Speed), "Speed")
	Expect(a.Major).To(Equal(b.Major), "Major")
	Expect(a.Minor).To(Equal(b.Minor), "Minor")
	Expect(a.BDeviceClass).To(Equal(b.BDeviceClass), "BDeviceClass")
	Expect(a.BDeviceSubClass).To(Equal(b.BDeviceSubClass), "BDeviceSubClass")
	Expect(a.BDeviceProtocol).To(Equal(b.BDeviceProtocol), "BDeviceProtocol")
	Expect(a.BConfigurationValue).To(Equal(b.BConfigurationValue), "BConfigurationValue")
	Expect(a.BNumConfigurations).To(Equal(b.BNumConfigurations), "BNumConfigurations")
	Expect(a.BNumInterfaces).To(Equal(b.BNumInterfaces), "BNumInterfaces")
	Expect(a.Interfaces).To(Equal(b.Interfaces), "Interfaces")
}

var (
	testUsb = USBDevice{
		Path:                "testdata/sys/bus/usb/devices/5-1",
		BusID:               "5-1",
		Manufacturer:        "Myself",
		Product:             "VirtualBlockDevice",
		Serial:              "123",
		DevicePath:          "/dev/bus/usb/005/002",
		Driver:              "usb",
		IsHub:               false,
		VendorID:            2385,
		ProductID:           260,
		BCD:                 1544,
		Bus:                 5,
		DeviceNumber:        2,
		Speed:               480,
		Major:               189,
		Minor:               513,
		BDeviceClass:        0,
		BDeviceSubClass:     0,
		BDeviceProtocol:     0,
		BConfigurationValue: 1,
		BNumConfigurations:  1,
		BNumInterfaces:      1,
		Interfaces: []USBDeviceInterface{
			{
				BInterfaceClass:    8,
				BInterfaceSubClass: 6,
				BInterfaceProtocol: 80,
			},
		},
	}

	testUsbHub = USBDevice{
		Path:                "testdata/sys/bus/usb/devices/usb1",
		BusID:               "usb1",
		Manufacturer:        "Linux 6.8.0-90-generic xhci-hcd",
		Product:             "xHCI Host Controller",
		Serial:              "0000:00:14.0",
		DevicePath:          "/dev/bus/usb/001/001",
		Driver:              "usb",
		IsHub:               true,
		VendorID:            7531,
		ProductID:           2,
		BCD:                 1544,
		Bus:                 1,
		DeviceNumber:        1,
		Speed:               480,
		Major:               189,
		Minor:               128,
		BDeviceClass:        9,
		BDeviceSubClass:     0,
		BDeviceProtocol:     1,
		BConfigurationValue: 1,
		BNumConfigurations:  1,
		BNumInterfaces:      1,
	}
)
