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

package watcher

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("ResourceSliceWatcher", func() {
	Describe("nodePCIDeviceNamesFromSlice", func() {
		It("returns normalized unique PCI device names", func() {
			resourceSlice := &resourcev1.ResourceSlice{
				Spec: resourcev1.ResourceSliceSpec{
					Devices: []resourcev1.Device{
						{Name: "gpu-1"},
						{Name: "pci-1"},
						{
							Name: "pci-2",
							Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
								"name": {StringValue: ptr.To("PCI.Device.2")},
							},
						},
						{
							Name: "pci-duplicate",
							Attributes: map[resourcev1.QualifiedName]resourcev1.DeviceAttribute{
								"name": {StringValue: ptr.To("PCI.Device.2")},
							},
						},
					},
				},
			}

			actual := nodePCIDeviceNamesFromSlice(resourceSlice)

			Expect(actual).To(Equal([]string{"pci-1", "pci-device-2"}))
		})
	})

	Describe("nodePCIDeviceName", func() {
		It("skips non-pci devices", func() {
			name, ok := nodePCIDeviceName(resourcev1.Device{Name: "gpu-1"})

			Expect(ok).To(BeFalse())
			Expect(name).To(BeEmpty())
		})

		It("returns the raw pci name", func() {
			name, ok := nodePCIDeviceName(resourcev1.Device{Name: "pci-raw"})

			Expect(ok).To(BeTrue())
			Expect(name).To(Equal("pci-raw"))
		})
	})
})
