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

package internal

import (
	"strings"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	// USBDeviceNamePrefix is the prefix for USB device names in ResourceSlice.
	USBDeviceNamePrefix = "usb-"
)

// IsUSBDevice checks if the device name has USB device prefix.
func IsUSBDevice(device resourcev1beta1.Device) bool {
	return strings.HasPrefix(device.Name, USBDeviceNamePrefix)
}

// ConvertDeviceToAttributes converts ResourceSlice device to NodeUSBDeviceAttributes.
func ConvertDeviceToAttributes(device resourcev1beta1.Device, nodeName string) v1alpha2.NodeUSBDeviceAttributes {
	attrs := v1alpha2.NodeUSBDeviceAttributes{
		NodeName: nodeName,
		Name:     device.Name,
	}

	if device.Basic == nil {
		return attrs
	}

	for key, attr := range device.Basic.Attributes {
		switch string(key) {
		case "name":
			if attr.StringValue != nil {
				attrs.Name = *attr.StringValue
			}
		case "manufacturer":
			if attr.StringValue != nil {
				attrs.Manufacturer = *attr.StringValue
			}
		case "product":
			if attr.StringValue != nil {
				attrs.Product = *attr.StringValue
			}
		case "vendorID":
			if attr.StringValue != nil {
				attrs.VendorID = *attr.StringValue
			}
		case "productID":
			if attr.StringValue != nil {
				attrs.ProductID = *attr.StringValue
			}
		case "bcd":
			if attr.StringValue != nil {
				attrs.BCD = *attr.StringValue
			}
		case "bus":
			if attr.StringValue != nil {
				attrs.Bus = *attr.StringValue
			}
		case "deviceNumber":
			if attr.StringValue != nil {
				attrs.DeviceNumber = *attr.StringValue
			}
		case "serial":
			if attr.StringValue != nil {
				attrs.Serial = *attr.StringValue
			}
		case "devicePath":
			if attr.StringValue != nil {
				attrs.DevicePath = *attr.StringValue
			}
		case "major":
			if attr.IntValue != nil {
				attrs.Major = int(*attr.IntValue)
			}
		case "minor":
			if attr.IntValue != nil {
				attrs.Minor = int(*attr.IntValue)
			}
		}
	}

	return attrs
}

// FindDeviceInSlices searches for a device with the given name in ResourceSlices.
// Returns the device attributes and true if found, empty attributes and false otherwise.
// Device name is unique and guaranteed by the DRA driver.
func FindDeviceInSlices(slices []resourcev1beta1.ResourceSlice, deviceName, nodeName string) (v1alpha2.NodeUSBDeviceAttributes, bool) {
	for _, slice := range slices {
		if slice.Spec.Pool.Name != nodeName {
			continue
		}

		for _, device := range slice.Spec.Devices {
			if !IsUSBDevice(device) {
				continue
			}
			if device.Name != deviceName {
				continue
			}

			return ConvertDeviceToAttributes(device, nodeName), true
		}
	}

	return v1alpha2.NodeUSBDeviceAttributes{}, false
}

// DeviceExistsInSlices checks if a device with the given name exists in ResourceSlices.
func DeviceExistsInSlices(slices []resourcev1beta1.ResourceSlice, deviceName, nodeName string) bool {
	_, found := FindDeviceInSlices(slices, deviceName, nodeName)
	return found
}
