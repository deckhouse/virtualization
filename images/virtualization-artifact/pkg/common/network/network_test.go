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

package network

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network config generation suite")
}

var _ = Describe("Network Config Generation", func() {
	var vmSpec virtv2.VirtualMachineSpec
	var vmmacs []*virtv2.VirtualMachineMACAddress

	newMACAddress := func(name, address string, phase virtv2.VirtualMachineMACAddressPhase, attachedVM string) *virtv2.VirtualMachineMACAddress {
		mac := &virtv2.VirtualMachineMACAddress{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineMACAddress",
				APIVersion: "virtualization.deckhouse.io/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "ns",
			},
			Status: virtv2.VirtualMachineMACAddressStatus{
				Address: address,
			},
		}
		if phase != "" {
			mac.Status.Phase = phase
		}
		if attachedVM != "" {
			mac.Status.VirtualMachine = attachedVM
		}
		return mac
	}

	BeforeEach(func() {
		vmSpec = virtv2.VirtualMachineSpec{
			Networks: []virtv2.NetworksSpec{},
		}
		vmmac1 := newMACAddress("mac1", "00:1A:2B:3C:4D:5E", virtv2.VirtualMachineMACAddressPhaseBound, "vm1")
		vmmac2 := newMACAddress("mac2", "00:1A:2B:3C:4D:5F", virtv2.VirtualMachineMACAddressPhaseBound, "vm2")
		vmmacs = []*virtv2.VirtualMachineMACAddress{vmmac1, vmmac2}
	})

	It("should return empty list interfaces", func() {
		vmSpec.Networks = []virtv2.NetworksSpec{
			{
				Type: virtv2.NetworksTypeMain,
			},
		}

		configs := CreateNetworkSpec(vmSpec, vmmacs)

		Expect(configs).To(HaveLen(0))
	})

	It("should generate correct interface name for Network type", func() {
		vmSpec.Networks = []virtv2.NetworksSpec{
			{
				Type: virtv2.NetworksTypeMain,
			},
			{
				Type: virtv2.NetworksTypeNetwork,
				Name: "mynet",
			},
		}

		configs := CreateNetworkSpec(vmSpec, vmmacs)

		Expect(configs).To(HaveLen(1))
		Expect(configs[0].Type).To(Equal(virtv2.NetworksTypeNetwork))
		Expect(configs[0].Name).To(Equal("mynet"))
		Expect(configs[0].InterfaceName).To(HavePrefix("veth_n"))
	})

	It("should generate correct interface name for ClusterNetwork type", func() {
		vmSpec.Networks = []virtv2.NetworksSpec{
			{
				Type: virtv2.NetworksTypeMain,
			},
			{
				Type: virtv2.NetworksTypeClusterNetwork,
				Name: "clusternet",
			},
		}

		configs := CreateNetworkSpec(vmSpec, vmmacs)

		Expect(configs).To(HaveLen(1))
		Expect(configs[0].Type).To(Equal(virtv2.NetworksTypeClusterNetwork))
		Expect(configs[0].Name).To(Equal("clusternet"))
		Expect(configs[0].InterfaceName).To(HavePrefix("veth_cn"))
	})

	It("should generate unique names for different networks with same name and id", func() {
		vmSpec.Networks = []virtv2.NetworksSpec{
			{
				Type: virtv2.NetworksTypeMain,
			},
			{
				Type: virtv2.NetworksTypeNetwork,
				Name: "net1",
			},
			{
				Type: virtv2.NetworksTypeNetwork,
				Name: "net1",
			},
		}

		configs := CreateNetworkSpec(vmSpec, vmmacs)

		Expect(configs).To(HaveLen(2))
		Expect(configs[0].InterfaceName).NotTo(Equal(configs[1].InterfaceName))
	})
})
