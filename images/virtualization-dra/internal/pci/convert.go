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

package pci

import (
	"log/slog"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/dynamic-resource-allocation/deviceattribute"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-dra/internal/consts"
)

func (d *Device) ToAPIDevice(nodeName string) *resourcev1.Device {
	name := d.GetName(nodeName)

	// The device carries no nodeName: the ResourceSlice publisher owns the
	// slices by the Node object and sets the slice-level nodeName itself;
	// a per-device nodeName is rejected without perDeviceNodeSelection.
	device := &resourcev1.Device{
		Name: name,
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
			consts.AttrName: {
				StringValue: ptr.To(name),
			},
			consts.AttrVendorID: {
				StringValue: ptr.To(d.VendorID),
			},
			consts.AttrDeviceID: {
				StringValue: ptr.To(d.DeviceID),
			},
			consts.AttrClass: {
				StringValue: ptr.To(d.ClassCode),
			},
			consts.AttrIOMMUGroup: {
				IntValue: ptr.To(int64(d.IOMMUGroup)),
			},
			consts.AttrDriver: {
				StringValue: ptr.To(d.Driver),
			},
			consts.AttrStandardPCIBusID: {
				StringValue: ptr.To(d.Address),
			},
		},
	}

	if d.NUMANode >= 0 {
		device.Attributes[consts.AttrStandardNUMANode] = resourcev1.DeviceAttribute{
			IntValue: ptr.To(d.NUMANode),
		}
	}

	if pcieRoot, err := deviceattribute.GetPCIeRootAttributeByPCIBusID(d.Address); err != nil {
		slog.Warn("Failed to resolve PCIe root complex for device", slog.String("address", d.Address), slog.Any("err", err))
	} else {
		device.Attributes[pcieRoot.Name] = pcieRoot.Value
	}

	return device
}
