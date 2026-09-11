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
	"testing"

	resourcev1 "k8s.io/api/resource/v1"
)

func TestIsPCIDevice(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		expectPCI  bool
	}{
		{name: "pci device", deviceName: "pci-device-1", expectPCI: true},
		{name: "non pci device", deviceName: "usb-device-1", expectPCI: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := resourcev1.Device{Name: tt.deviceName}
			if IsPCIDevice(device) != tt.expectPCI {
				t.Fatalf("expected %v for device %q", tt.expectPCI, tt.deviceName)
			}
		})
	}
}

func TestConvertDeviceToAttributes(t *testing.T) {
	device := resourcev1.Device{
		Name: "pci-device-1",
		Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
			"name":                            {StringValue: ptrString("pci-device-1")},
			"vendorID":                        {StringValue: ptrString("10de")},
			"deviceID":                        {StringValue: ptrString("2204")},
			"class":                           {StringValue: ptrString("0300")},
			"driver":                          {StringValue: ptrString("vfio-pci")},
			"iommuGroup":                      {IntValue: ptrInt64(42)},
			"resource.kubernetes.io/pciBusID": {StringValue: ptrString("0000:3b:00.0")},
			"resource.kubernetes.io/numaNode": {IntValue: ptrInt64(1)},
		},
	}

	attrs := ConvertDeviceToAttributes(device, "node-a")

	if attrs.NodeName != "node-a" {
		t.Fatalf("expected node name node-a, got %q", attrs.NodeName)
	}
	if attrs.PCIAddress != "0000:3b:00.0" {
		t.Fatalf("expected pci address 0000:3b:00.0, got %q", attrs.PCIAddress)
	}
	if attrs.VendorID != "10de" || attrs.DeviceID != "2204" || attrs.Class != "0300" || attrs.Driver != "vfio-pci" {
		t.Fatalf("unexpected attributes: %+v", attrs)
	}
	if attrs.IOMMUGroup != 42 {
		t.Fatalf("expected iommu group 42, got %d", attrs.IOMMUGroup)
	}
	if attrs.NUMANode != 1 {
		t.Fatalf("expected numa node 1, got %d", attrs.NUMANode)
	}
}

func TestConvertDeviceToAttributesWithoutNUMANode(t *testing.T) {
	device := resourcev1.Device{Name: "pci-device-1"}

	attrs := ConvertDeviceToAttributes(device, "node-a")

	if attrs.NUMANode != -1 {
		t.Fatalf("expected numa node -1, got %d", attrs.NUMANode)
	}
}

func ptrString(v string) *string { return &v }

func ptrInt64(v int64) *int64 { return &v }
