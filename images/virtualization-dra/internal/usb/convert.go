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

	"github.com/deckhouse/virtualization-dra/internal/consts"
)

func (d *Device) ToAPIDevice(nodeName string) *resourcev1.Device {
	return convertToAPIDevice(*d, nodeName)
}

func convertToAPIDevice(usbDevice Device, nodeName string) *resourcev1.Device {
	name := usbDevice.GetName(nodeName)
	device := &resourcev1.Device{
		Name: name,
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
			consts.AttrName: {
				StringValue: ptr.To(name),
			},
			consts.AttrPath: {
				StringValue: ptr.To(usbDevice.Path),
			},
			consts.AttrBusID: {
				StringValue: ptr.To(usbDevice.BusID),
			},
			consts.AttrManufacturer: {
				StringValue: ptr.To(usbDevice.Manufacturer),
			},
			consts.AttrProduct: {
				StringValue: ptr.To(usbDevice.Product),
			},
			consts.AttrVendorID: {
				StringValue: ptr.To(usbDevice.VendorID.String()),
			},
			consts.AttrProductID: {
				StringValue: ptr.To(usbDevice.ProductID.String()),
			},
			consts.AttrBCD: {
				StringValue: ptr.To(usbDevice.BCD.String()),
			},
			consts.AttrBus: {
				StringValue: ptr.To(usbDevice.Bus.String()),
			},
			consts.AttrDeviceNumber: {
				StringValue: ptr.To(usbDevice.DeviceNumber.String()),
			},
			consts.AttrMajor: {
				IntValue: ptr.To(int64(usbDevice.Major)),
			},
			consts.AttrMinor: {
				IntValue: ptr.To(int64(usbDevice.Minor)),
			},
			consts.AttrSerial: {
				StringValue: ptr.To(usbDevice.Serial),
			},
			consts.AttrDevicePath: {
				StringValue: ptr.To(usbDevice.DevicePath),
			},
			consts.AttrUsbAddress: {
				StringValue: ptr.To(fmt.Sprintf("%s:%s", usbDevice.Bus, usbDevice.DeviceNumber)),
			},
		},
	}

	return device
}
