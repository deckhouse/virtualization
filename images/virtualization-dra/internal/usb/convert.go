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
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/utils/ptr"
)

func convertToAPIDevice(usbDevice Device) *resourceapi.Device {
	return &resourceapi.Device{
		Name: usbDevice.GetName(),
		Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
			"name": {
				StringValue: ptr.To(usbDevice.Name),
			},
			"manufacturer": {
				StringValue: ptr.To(usbDevice.Manufacturer),
			},
			"vendorID": {
				StringValue: ptr.To(usbDevice.VendorID.String()),
			},
			"productID": {
				StringValue: ptr.To(usbDevice.ProductID.String()),
			},
			"bcd": {
				StringValue: ptr.To(usbDevice.BCD.String()),
			},
			"bus": {
				StringValue: ptr.To(usbDevice.Bus.String()),
			},
			"resource.kubernetes.io/usbAddressBus": {
				IntValue: ptr.To(int64(usbDevice.Bus)),
			},
			"deviceNumber": {
				StringValue: ptr.To(usbDevice.DeviceNumber.String()),
			},
			"resource.kubernetes.io/usbAddressDeviceNumber": {
				IntValue: ptr.To(int64(usbDevice.DeviceNumber)),
			},
			"major": {
				IntValue: ptr.To(int64(usbDevice.Major)),
			},
			"minor": {
				IntValue: ptr.To(int64(usbDevice.Minor)),
			},
			"serial": {
				StringValue: ptr.To(usbDevice.Serial),
			},
			"devicePath": {
				StringValue: ptr.To(usbDevice.DevicePath),
			},
		},
	}
}
