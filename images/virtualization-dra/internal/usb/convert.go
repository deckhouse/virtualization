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

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-dra/internal/common"
)

func (d *Device) ToAPIDevice(nodeName string) *resourcev1.Device {
	return convertToAPIDevice(*d, nodeName)
}

func convertToAPIDevice(usbDevice Device, nodeName string) *resourcev1.Device {
	name := usbDevice.GetName(nodeName)
	device := &resourcev1.Device{
		Name: name,
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
			common.AttrName: {
				StringValue: ptr.To(name),
			},
			common.AttrPath: {
				StringValue: ptr.To(usbDevice.Path),
			},
			common.AttrBusID: {
				StringValue: ptr.To(usbDevice.BusID),
			},
			common.AttrManufacturer: {
				StringValue: ptr.To(usbDevice.Manufacturer),
			},
			common.AttrProduct: {
				StringValue: ptr.To(usbDevice.Product),
			},
			common.AttrVendorID: {
				StringValue: ptr.To(usbDevice.VendorID.String()),
			},
			common.AttrProductID: {
				StringValue: ptr.To(usbDevice.ProductID.String()),
			},
			common.AttrBCD: {
				StringValue: ptr.To(usbDevice.BCD.String()),
			},
			common.AttrBus: {
				StringValue: ptr.To(usbDevice.Bus.String()),
			},
			common.AttrDeviceNumber: {
				StringValue: ptr.To(usbDevice.DeviceNumber.String()),
			},
			common.AttrMajor: {
				IntValue: ptr.To(int64(usbDevice.Major)),
			},
			common.AttrMinor: {
				IntValue: ptr.To(int64(usbDevice.Minor)),
			},
			common.AttrSerial: {
				StringValue: ptr.To(usbDevice.Serial),
			},
			common.AttrDevicePath: {
				StringValue: ptr.To(usbDevice.DevicePath),
			},
			common.AttrUsbAddress: {
				StringValue: ptr.To(fmt.Sprintf("%s:%s", usbDevice.Bus, usbDevice.DeviceNumber)),
			},
		},
	}

	return device
}
