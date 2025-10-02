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

package service

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("MACAddressService", func() {
	var (
		service       *MACAddressService
		allocatedMACs mac.AllocatedMACs
	)

	BeforeEach(func() {
		allocatedMACs = make(mac.AllocatedMACs)
		service = NewMACAddressService("17eb5ee6-192c-4048-b79e-a21eaf8b7121", nil, nil)
	})

	Context("generateOUI", func() {
		It("should return empty string when clusterUID is empty", func() {
			oui := generateOUI("")
			Expect(oui).To(BeEmpty())
		})
		It("should return empty string when clusterUID is not a valid UUID", func() {
			oui := generateOUI("invalid-uuid")
			Expect(oui).To(BeEmpty())
		})
		It("should return OUI string when clusterUID is a valid UUID", func() {
			oui := generateOUI("17eb5ee6-192c-4048-b79e-a21eaf8b7121")
			Expect(oui).To(Equal("5ee619"))

			oui2 := generateOUI("8831e5c6-92d1-4eb8-bb85-034a708e22a6")
			Expect(oui2).To(Equal("c692d1"))

			oui3 := generateOUI("b7e15fbe-ca16-4639-a3f5-8a77a1b68a90")
			Expect(oui3).To(Equal("beca16"))

			oui4 := generateOUI("d1cb2c60-90da-4d6e-83ad-1fd69dba3d0f")
			Expect(oui4).To(Equal("da4d6e"))

			oui5 := generateOUI("f2c41929-04e4-4e81-bb88-6e8e89ab5143")
			Expect(oui5).To(Equal("f2c419"))
		})

		Context("IsAvailableAddress", func() {
			It("should return error for an invalid MAC address format", func() {
				err := service.IsAvailableAddress("invalid-mac", allocatedMACs)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("invalid MAC address format"))
			})

			It("should return error for a duplicate MAC address", func() {
				ref := v1alpha2.VirtualMachineMACAddressLeaseMACAddressRef{
					Name:      "test",
					Namespace: "test",
				}

				spec := v1alpha2.VirtualMachineMACAddressLeaseSpec{
					VirtualMachineMACAddressRef: &ref,
				}

				lease := &v1alpha2.VirtualMachineMACAddressLease{
					Spec: spec,
				}

				allocatedMACs["f6:e1:74:94:AB:CD"] = lease
				err := service.IsAvailableAddress("f6:e1:74:94:AB:CD", allocatedMACs)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(ErrMACAddressAlreadyExist))
			})

			It("should return nil for a valid MAC address", func() {
				err := service.IsAvailableAddress("f6:e1:74:94:12:34", allocatedMACs)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("AllocateNewAddress", func() {
			It("should allocate a new unique MAC address with format oui xx-xx-xx-xx", func() {
				address, err := service.AllocateNewAddress(allocatedMACs)
				Expect(err).NotTo(HaveOccurred())
				Expect(address).To(HavePrefix("5e:e6:19"))
			})
		})
	})
})
