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

package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	resourcev1beta1 "k8s.io/api/resource/v1beta1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// CalculateHash calculates hash for USB device attributes.
func CalculateHash(attrs v1alpha2.NodeUSBDeviceAttributes) string {
	hashInput := fmt.Sprintf("%s:%s:%s:%s:%s:%s:%s",
		attrs.NodeName,
		attrs.VendorID,
		attrs.ProductID,
		attrs.Bus,
		attrs.DeviceNumber,
		attrs.Serial,
		attrs.DevicePath,
	)

	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:])[:16] // Use first 16 characters
}

// CalculateHashFromDevice calculates hash from ResourceSlice Device.
func CalculateHashFromDevice(device resourcev1beta1.Device, nodeName string) string {
	var vendorID, productID, bus, deviceNumber, serial, devicePath string

	if device.Basic != nil {
		for key, attr := range device.Basic.Attributes {
			switch string(key) {
			case "vendorID":
				if attr.StringValue != nil {
					vendorID = *attr.StringValue
				}
			case "productID":
				if attr.StringValue != nil {
					productID = *attr.StringValue
				}
			case "bus":
				if attr.StringValue != nil {
					bus = *attr.StringValue
				}
			case "deviceNumber":
				if attr.StringValue != nil {
					deviceNumber = *attr.StringValue
				}
			case "serial":
				if attr.StringValue != nil {
					serial = *attr.StringValue
				}
			case "devicePath":
				if attr.StringValue != nil {
					devicePath = *attr.StringValue
				}
			}
		}
	}

	attrs := v1alpha2.NodeUSBDeviceAttributes{
		NodeName:    nodeName,
		VendorID:    vendorID,
		ProductID:   productID,
		Bus:         bus,
		DeviceNumber: deviceNumber,
		Serial:      serial,
		DevicePath:  devicePath,
	}

	return CalculateHash(attrs)
}
