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

package libusb

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Device discovery", func() {
	BeforeEach(func() {
		pathToUSBDevices = "testdata/sys/bus/usb/devices"
	})

	It("should discover plugged usb devices, ignore hubs and invalid devices", func() {
		devices, err := DiscoverPluggedUSBDevices()
		Expect(err).ToNot(HaveOccurred())

		Expect(devices).To(HaveLen(1))
		Expect(devices[testUsb.Path]).ToNot(BeNil())

		compareUsb(devices[testUsb.Path], &testUsb)
	})
})
