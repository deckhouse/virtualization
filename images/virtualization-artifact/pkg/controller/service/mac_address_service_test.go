/*
Copyright 2024 Flant JSC

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
	"fmt"

	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MACAddressService", func() {
	var (
		service       *MACAddressService
		allocatedMACs mac.AllocatedMACs
	)

	BeforeEach(func() {
		allocatedMACs = make(mac.AllocatedMACs)
		service = NewMACAddressService("f6:e1:74:94")
	})

	Context("IsAvailableAddress", func() {
		It("should return error for an invalid MAC address format", func() {
			err := service.IsAvailableAddress("invalid-mac", allocatedMACs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("invalid MAC address format"))
		})

		It("should return error for a duplicate MAC address", func() {
			ref := virtv2.VirtualMachineMACAddressLeaseMACAddressRef{
				Name:      "test",
				Namespace: "test",
			}

			spec := virtv2.VirtualMachineMACAddressLeaseSpec{
				VirtualMachineMACAddressRef: &ref,
			}

			lease := &virtv2.VirtualMachineMACAddressLease{
				Spec: spec,
			}

			allocatedMACs["f6:e1:74:94:AB:CD"] = lease
			err := service.IsAvailableAddress("f6:e1:74:94:AB:CD", allocatedMACs)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(ErrMACAddressAlreadyExist))
		})

		It("should return error for a MAC address out of prefix range", func() {
			err := service.IsAvailableAddress("00:11:22:33:44:55", allocatedMACs)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(ErrMACAddressOutOfRange))
		})

		It("should return nil for a valid MAC address", func() {
			err := service.IsAvailableAddress("f6:e1:74:94:12:34", allocatedMACs)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("AllocateNewAddress", func() {
		It("should allocate a new unique MAC address with format prefix xx:xx:xx:xx", func() {
			address, err := service.AllocateNewAddress(allocatedMACs)
			Expect(err).NotTo(HaveOccurred())
			Expect(address).To(HavePrefix("f6:e1:74:94"))
		})

		It("should allocate a new unique MAC address with format prefix xx-xx-xx-xx", func() {
			service := NewMACAddressService("f6-e1-74-94")
			address, err := service.AllocateNewAddress(allocatedMACs)
			Expect(err).NotTo(HaveOccurred())
			Expect(address).To(HavePrefix("f6:e1:74:94"))
		})

		It("should allocate a new unique MAC address with format prefix xxxxxxxx", func() {
			service := NewMACAddressService("f6e17494")
			address, err := service.AllocateNewAddress(allocatedMACs)
			Expect(err).NotTo(HaveOccurred())
			Expect(address).To(HavePrefix("f6:e1:74:94"))
		})

		It("should return an error when MAC addresses prefix wrong", func() {
			service := NewMACAddressService("f6e1749")
			_, err := service.AllocateNewAddress(allocatedMACs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("wrong format MAC address prefix"))
		})

		It("should return an error when no MAC addresses are available", func() {
			for i := 0; i < MaxCount; i++ {
				testRefName := fmt.Sprintf("test-%d", i)
				ref := virtv2.VirtualMachineMACAddressLeaseMACAddressRef{
					Name:      testRefName,
					Namespace: testRefName,
				}

				spec := virtv2.VirtualMachineMACAddressLeaseSpec{
					VirtualMachineMACAddressRef: &ref,
				}

				lease := &virtv2.VirtualMachineMACAddressLease{
					Spec: spec,
				}

				allocatedMACs[fmt.Sprintf("f6:e1:74:94:%02X:%02X", i/256, i%256)] = lease
			}

			address, err := service.AllocateNewAddress(allocatedMACs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("no remaining MAC addresses"))
			Expect(address).To(BeEmpty())
		})
	})
})
