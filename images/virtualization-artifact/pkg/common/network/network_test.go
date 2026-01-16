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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network config generation suite")
}

var _ = Describe("Network Config Generation", func() {
	vm := &v1alpha2.VirtualMachine{}
	var vmmacs []*v1alpha2.VirtualMachineMACAddress

	newMACAddress := func(name, address string, phase v1alpha2.VirtualMachineMACAddressPhase, attachedVM string) *v1alpha2.VirtualMachineMACAddress {
		mac := &v1alpha2.VirtualMachineMACAddress{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineMACAddress",
				APIVersion: "virtualization.deckhouse.io/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "ns",
			},
			Status: v1alpha2.VirtualMachineMACAddressStatus{
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
		vm.Spec.Networks = []v1alpha2.NetworksSpec{}
		vmmac1 := newMACAddress("mac1", "00:1A:2B:3C:4D:5E", v1alpha2.VirtualMachineMACAddressPhaseBound, "vm1")
		vmmac2 := newMACAddress("mac2", "00:1A:2B:3C:4D:5F", v1alpha2.VirtualMachineMACAddressPhaseBound, "vm2")
		vmmacs = []*v1alpha2.VirtualMachineMACAddress{vmmac1, vmmac2}
	})

	It("should return empty list interfaces", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
		}

		configs := CreateNetworkSpec(vm, vmmacs)

		Expect(configs).To(HaveLen(1))
		Expect(configs[0].Name).To(Equal(""))
		Expect(configs[0].InterfaceName).To(HavePrefix("default"))
		Expect(configs[0].MAC).To(HavePrefix(""))
	})

	It("should generate correct interface name for Network type", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "mynet",
			},
		}

		configs := CreateNetworkSpec(vm, vmmacs)

		Expect(configs).To(HaveLen(2))
		Expect(configs[0].Name).To(Equal(""))
		Expect(configs[0].InterfaceName).To(HavePrefix("default"))
		Expect(configs[0].MAC).To(HavePrefix(""))

		Expect(configs[1].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
		Expect(configs[1].Name).To(Equal("mynet"))
		Expect(configs[1].InterfaceName).To(HavePrefix("veth_n"))
	})

	It("should generate correct interface name for ClusterNetwork type", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
			{
				Type: v1alpha2.NetworksTypeClusterNetwork,
				Name: "clusternet",
			},
		}

		configs := CreateNetworkSpec(vm, vmmacs)

		Expect(configs).To(HaveLen(2))
		Expect(configs[0].Name).To(Equal(""))
		Expect(configs[0].InterfaceName).To(HavePrefix("default"))
		Expect(configs[0].MAC).To(HavePrefix(""))
		Expect(configs[1].Type).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
		Expect(configs[1].Name).To(Equal("clusternet"))
		Expect(configs[1].InterfaceName).To(HavePrefix("veth_cn"))
	})

	It("should generate unique names for different networks", func() {
		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "net1",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "net1",
			},
		}

		configs := CreateNetworkSpec(vm, vmmacs)

		Expect(configs).To(HaveLen(3))
		Expect(configs[0].Name).To(Equal(""))
		Expect(configs[0].InterfaceName).To(HavePrefix("default"))
		Expect(configs[0].MAC).To(HavePrefix(""))
		Expect(configs[1].InterfaceName).NotTo(Equal(configs[2].InterfaceName))
	})

	It("should preserve MAC order for existing networks and assign free MAC to new network", func() {
		vm.Status.Networks = []v1alpha2.NetworksStatus{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
				MAC:  "00:1A:2B:3C:4D:5E",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
				MAC:  "00:1A:2B:3C:4D:5F",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
				MAC:  "00:1A:2B:3C:4D:6A",
			},
		}

		vmmac1 := newMACAddress("mac1", "00:1A:2B:3C:4D:5E", v1alpha2.VirtualMachineMACAddressPhaseAttached, "vm1")
		vmmac2 := newMACAddress("mac2", "00:1A:2B:3C:4D:5F", v1alpha2.VirtualMachineMACAddressPhaseAttached, "vm1")
		vmmac3 := newMACAddress("mac3", "00:1A:2B:3C:4D:6A", v1alpha2.VirtualMachineMACAddressPhaseAttached, "vm1")
		vmmac4 := newMACAddress("mac4", "00:1A:2B:3C:4D:7F", v1alpha2.VirtualMachineMACAddressPhaseAttached, "vm1")
		vmmacs = append(vmmacs, vmmac1, vmmac2, vmmac3, vmmac4)

		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name2",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
			},
		}

		configs := CreateNetworkSpec(vm, vmmacs)

		Expect(configs).To(HaveLen(5))

		Expect(configs[0].Name).To(Equal(""))
		Expect(configs[0].MAC).To(Equal(""))

		Expect(configs[1].Name).To(Equal("name1"))
		Expect(configs[1].MAC).To(Equal("00:1A:2B:3C:4D:5E"))

		Expect(configs[2].Name).To(Equal("name1"))
		Expect(configs[2].MAC).To(Equal("00:1A:2B:3C:4D:5F"))

		Expect(configs[3].Name).To(Equal("name2"))
		Expect(configs[3].MAC).To(Equal("00:1A:2B:3C:4D:7F"))

		Expect(configs[4].Name).To(Equal("name1"))
		Expect(configs[4].MAC).To(Equal("00:1A:2B:3C:4D:6A"))
	})

	It("should preserve MAC order when delete network", func() {
		vm.Status.Networks = []v1alpha2.NetworksStatus{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
				MAC:  "00:1A:2B:3C:4D:5E",
			},
			{
				Name: "name1",
				MAC:  "00:1A:2B:3C:4D:5F",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name2",
				MAC:  "00:1A:2B:3C:4D:7F",
			},
			{
				Name: "name1",
				MAC:  "00:1A:2B:3C:4D:6A",
			},
		}

		vmmac1 := newMACAddress("mac1", "00:1A:2B:3C:4D:5E", v1alpha2.VirtualMachineMACAddressPhaseAttached, "vm1")
		vmmac2 := newMACAddress("mac2", "00:1A:2B:3C:4D:5F", v1alpha2.VirtualMachineMACAddressPhaseAttached, "vm1")
		vmmac3 := newMACAddress("mac3", "00:1A:2B:3C:4D:6A", v1alpha2.VirtualMachineMACAddressPhaseAttached, "vm1")
		vmmac4 := newMACAddress("mac4", "00:1A:2B:3C:4D:7F", v1alpha2.VirtualMachineMACAddressPhaseAttached, "vm1")
		vmmacs = append(vmmacs, vmmac1, vmmac2, vmmac3, vmmac4)

		vm.Spec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "name1",
			},
		}

		configs := CreateNetworkSpec(vm, vmmacs)

		Expect(configs).To(HaveLen(4))

		Expect(configs[1].Name).To(Equal("name1"))
		Expect(configs[1].MAC).To(Equal("00:1A:2B:3C:4D:5E"))

		Expect(configs[2].Name).To(Equal("name1"))
		Expect(configs[2].MAC).To(Equal("00:1A:2B:3C:4D:5F"))

		Expect(configs[3].Name).To(Equal("name1"))
		Expect(configs[3].MAC).To(Equal("00:1A:2B:3C:4D:6A"))
	})
})
