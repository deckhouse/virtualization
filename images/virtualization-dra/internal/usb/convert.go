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
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-dra/internal/common"
	"github.com/deckhouse/virtualization-dra/internal/featuregates"
)

func convertToAPIDevice(usbDevice Device, nodeName string) *resourceapi.Device {
	name := usbDevice.GetName(nodeName)
	device := &resourceapi.Device{
		Name: name,
		Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
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
			common.AttrVendorId: {
				StringValue: ptr.To(usbDevice.VendorID.String()),
			},
			common.AttrProductId: {
				StringValue: ptr.To(usbDevice.ProductID.String()),
			},
			common.AttrBCD: {
				StringValue: ptr.To(usbDevice.BCD.String()),
			},
			common.AttrBus: {
				StringValue: ptr.To(usbDevice.Bus.String()),
			},
			common.AttrUsbAddressBus: {
				IntValue: ptr.To(int64(usbDevice.Bus)),
			},
			common.AttrDeviceNumber: {
				StringValue: ptr.To(usbDevice.DeviceNumber.String()),
			},
			common.AttrUsbAddressDeviceNumber: {
				IntValue: ptr.To(int64(usbDevice.DeviceNumber)),
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
		},
	}

	if !featuregates.Default().USBGatewayEnabled() {
		device.NodeName = ptr.To(nodeName)
	}

	return device
}

func getNodeSelector() *corev1.NodeSelector {
	return &corev1.NodeSelector{
		NodeSelectorTerms: []corev1.NodeSelectorTerm{
			{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Key:      common.USBGatewayLabel,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"true"},
					},
				},
			},
		},
	}
}
