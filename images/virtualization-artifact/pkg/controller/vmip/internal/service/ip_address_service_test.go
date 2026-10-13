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
	"context"
	"net/netip"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("IsInsideOfRange", func() {
	var ipService *IPAddressService

	BeforeEach(func() {
		virtualMachineCIDRs := []string{"192.168.1.0/24"}
		var err error
		ipService, err = NewIPAddressService(virtualMachineCIDRs, nil, nil)
		Expect(err).To(BeNil())
	})

	Describe("IsAvailableAddress", func() {
		Context("with a valid and available IP address", func() {
			It("should return no error", func() {
				err := ipService.IsInsideOfRange("192.168.1.10")
				Expect(err).To(BeNil())
			})
		})

		Context("with an invalid IP address", func() {
			It("should return an error", func() {
				err := ipService.IsInsideOfRange("invalid-ip")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with an IP address out of range", func() {
			It("should return ErrIPAddressOutOfRange", func() {
				err := ipService.IsInsideOfRange("10.0.0.1")
				Expect(err).To(Equal(ErrIPAddressOutOfRange))
			})
		})
	})
})

var _ = Describe("AllocateNewIP", func() {
	var (
		ipService    *IPAddressService
		allocatedIPs ip.AllocatedIPs
	)

	BeforeEach(func() {
		virtualMachineCIDRs := []string{"192.168.1.0/24"}
		var err error
		ipService, err = NewIPAddressService(virtualMachineCIDRs, nil, nil)
		Expect(err).To(BeNil())
	})

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
			ipService, err := NewIPAddressService(virtualMachineCIDRs, nil, nil)
			Expect(err).To(BeNil())
			_, err = ipService.AllocateNewIP(allocatedIPs)
			Expect(err).To(MatchError("no remaining ips"))
		})
	})
})

var _ = Describe("GetAllocatedIPs", func() {
	newClientBuilder := func(objs ...client.Object) *fake.ClientBuilder {
		scheme := apiruntime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(sdnapi.AddToScheme(scheme)).To(Succeed())
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
	}

	newIPAddress := func(name, networkName, address string) *sdnapi.IPAddress {
		return &sdnapi.IPAddress{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vms"},
			Spec: sdnapi.IPAddressSpec{
				NetworkRef: sdnapi.NetworkRef{Kind: v1alpha2.NetworksTypeClusterNetwork, Name: networkName},
				Type:       sdnapi.IPAddressTypeAuto,
			},
			Status: sdnapi.IPAddressStatus{Address: address},
		}
	}

	It("merges vmip lease addresses with the IPAddresses of the main network", func() {
		// Both cover the same ranges during the transition, so an address the sdn
		// module has handed out must not be offered to a vmip.
		vmipl := &v1alpha2.VirtualMachineIPAddressLease{
			ObjectMeta: metav1.ObjectMeta{Name: ip.IPToLeaseName("192.168.1.3")},
		}
		cl := newClientBuilder(
			vmipl,
			newIPAddress("main-ip", commonnetwork.MainClusterNetworkName, "192.168.1.4"),
			newIPAddress("other-ip", "corp-net", "192.168.1.5"),
		).Build()

		allocatedIPs, err := IPAddressService{client: cl}.GetAllocatedIPs(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(allocatedIPs).To(HaveLen(2))
		Expect(allocatedIPs).To(HaveKey("192.168.1.3"))
		Expect(allocatedIPs).To(HaveKey("192.168.1.4"))
	})

	It("counts the static address of an IPAddress that is not allocated yet", func() {
		static := newIPAddress("static-ip", commonnetwork.MainClusterNetworkName, "")
		static.Spec.Type = sdnapi.IPAddressTypeStatic
		static.Spec.Static = &sdnapi.StaticIPAddress{IP: "192.168.1.7"}
		cl := newClientBuilder(static).Build()

		allocatedIPs, err := IPAddressService{client: cl}.GetAllocatedIPs(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(allocatedIPs).To(HaveKey("192.168.1.7"))
	})

	It("counts only the leases where the sdn module is not installed", func() {
		// A missing IPAddress kind means the sdn module is off, not an error.
		cl := newClientBuilder(&v1alpha2.VirtualMachineIPAddressLease{
			ObjectMeta: metav1.ObjectMeta{Name: ip.IPToLeaseName("192.168.1.3")},
		}).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*sdnapi.IPAddressList); ok {
						return &meta.NoKindMatchError{GroupKind: sdnapi.SchemeGroupVersion.WithKind("IPAddress").GroupKind()}
					}
					return c.List(ctx, list, opts...)
				},
			}).Build()

		allocatedIPs, err := IPAddressService{client: cl}.GetAllocatedIPs(context.Background())

		Expect(err).NotTo(HaveOccurred())
		Expect(allocatedIPs).To(HaveKey("192.168.1.3"))
	})
})
