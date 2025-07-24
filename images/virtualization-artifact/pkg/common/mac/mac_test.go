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

package mac

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMACAddressService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MAC Utilities Suite")
}

var _ = Describe("MAC Utilities", func() {
	Context("AddressToLeaseName", func() {
		It("should convert MAC address to Lease Name correctly", func() {
			address := "00:1A:2B:3C:4D:5E"
			expectedLeaseName := "mac-00-1a-2b-3c-4d-5e"
			Expect(AddressToLeaseName(address)).To(Equal(expectedLeaseName))
		})
	})

	Context("LeaseNameToAddress", func() {
		It("should convert Lease Name back to MAC address correctly", func() {
			leaseName := "mac-00-1a-2b-3c-4d-5e"
			expectedAddress := "00:1a:2b:3c:4d:5e"
			Expect(LeaseNameToAddress(leaseName)).To(Equal(expectedAddress))
		})

		It("should return an empty string for invalid Lease Name", func() {
			leaseName := "invalid-mac-name"
			Expect(LeaseNameToAddress(leaseName)).To(Equal(""))
		})
	})

	Context("IsValidAddressFormat", func() {
		It("should return true for a valid MAC address", func() {
			address := "00:1A:2B:3C:4D:5E"
			Expect(IsValidAddressFormat(address)).To(BeTrue())
		})

		It("should return false for an invalid MAC address", func() {
			address := "00:1G:2B:3C:4D:5E" // Invalid because 'G' is not a valid hex character
			Expect(IsValidAddressFormat(address)).To(BeFalse())
		})

		It("should return false for a MAC address with incorrect length", func() {
			addressShort := "00:1A:2B:3C:4D"
			addressLong := "00:1A:2B:3C:4D:5E:6F"
			Expect(IsValidAddressFormat(addressShort)).To(BeFalse())
			Expect(IsValidAddressFormat(addressLong)).To(BeFalse())
		})

		It("should return false for a MAC address with special characters", func() {
			address := "00:1A:2B:3C:4D:5E!"
			Expect(IsValidAddressFormat(address)).To(BeFalse())
		})
	})
})
