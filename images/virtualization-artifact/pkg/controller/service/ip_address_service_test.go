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
	"net/netip"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("IpAddressService", func() {
	var (
		ipService    *IpAddressService
		allocatedIPs ip.AllocatedIPs
		logger       *log.Logger
	)

	BeforeEach(func() {
		virtualMachineCIDRs := []string{"192.168.1.0/24"}
		ipService = NewIpAddressService(logger, virtualMachineCIDRs)
		allocatedIPs = make(ip.AllocatedIPs)
	})

	Describe("IsAvailableAddress", func() {
		Context("with a valid and available IP address", func() {
			It("should return no error", func() {
				err := ipService.IsAvailableAddress("192.168.1.10", allocatedIPs)
				Expect(err).To(BeNil())
			})
		})

		Context("with an invalid IP address", func() {
			It("should return an error", func() {
				err := ipService.IsAvailableAddress("invalid-ip", allocatedIPs)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with an already allocated IP address", func() {
			It("should return ErrIPAddressAlreadyExist", func() {
				ref := virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
					Name:      "test",
					Namespace: "test",
				}

				spec := virtv2.VirtualMachineIPAddressLeaseSpec{
					VirtualMachineIPAddressRef: &ref,
				}

				lease := &virtv2.VirtualMachineIPAddressLease{
					Spec: spec,
				}

				allocatedIPs["192.168.1.10"] = lease
				err := ipService.IsAvailableAddress("192.168.1.10", allocatedIPs)
				Expect(err).To(Equal(ErrIPAddressAlreadyExist))
			})
		})

		Context("with an IP address out of range", func() {
			It("should return ErrIPAddressOutOfRange", func() {
				err := ipService.IsAvailableAddress("10.0.0.1", allocatedIPs)
				Expect(err).To(Equal(ErrIPAddressOutOfRange))
			})
		})
	})

	Describe("AllocateNewAddress", func() {
		Context("when there are available IP addresses in the range", func() {
			It("should allocate a new IP address", func() {
				ip, err := ipService.AllocateNewIP(allocatedIPs)
				Expect(err).To(BeNil())
				Expect(ip).ToNot(BeEmpty())
				Expect(netip.MustParseAddr(ip).IsValid()).To(BeTrue())
			})
		})

		Context("when there are no available IP addresses in the range", func() {
			It("should return an error", func() {
				virtualMachineCIDRs := []string{"192.168.1.0/31"}
				ipService := NewIpAddressService(logger, virtualMachineCIDRs)
				_, err := ipService.AllocateNewIP(allocatedIPs)
				Expect(err).To(MatchError("no remaining ips"))
			})
		})
	})
})
