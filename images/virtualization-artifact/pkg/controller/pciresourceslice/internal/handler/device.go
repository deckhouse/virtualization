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

package handler

import (
	"strings"

	resourcev1 "k8s.io/api/resource/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	PCIDeviceNamePrefix = "pci-"
)

func IsPCIDevice(device resourcev1.Device) bool {
	return strings.HasPrefix(device.Name, PCIDeviceNamePrefix)
}

func ConvertDeviceToAttributes(device resourcev1.Device, nodeName string) v1alpha2.NodePCIDeviceAttributes {
	attrs := v1alpha2.NodePCIDeviceAttributes{
		NodeName: nodeName,
		Name:     device.Name,
		NUMANode: -1,
	}

	for key, attr := range device.Attributes {
		switch string(key) {
		case "name":
			if attr.StringValue != nil {
				attrs.Name = *attr.StringValue
			}
		case "vendorID":
			if attr.StringValue != nil {
				attrs.VendorID = *attr.StringValue
			}
		case "deviceID":
			if attr.StringValue != nil {
				attrs.DeviceID = *attr.StringValue
			}
		case "class":
			if attr.StringValue != nil {
				attrs.Class = *attr.StringValue
			}
		case "driver":
			if attr.StringValue != nil {
				attrs.Driver = *attr.StringValue
			}
		case "iommuGroup":
			if attr.IntValue != nil {
				attrs.IOMMUGroup = *attr.IntValue
			}
		case "resource.kubernetes.io/pciBusID":
			if attr.StringValue != nil {
				attrs.PCIAddress = *attr.StringValue
			}
		case "resource.kubernetes.io/numaNode":
			if attr.IntValue != nil {
				attrs.NUMANode = *attr.IntValue
			}
		}
	}

	return attrs
}
