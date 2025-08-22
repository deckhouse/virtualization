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

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Network config generation suite")
}

var _ = Describe("Network Config Generation", func() {
	var vmSpec v1alpha2.VirtualMachineSpec

	BeforeEach(func() {
		vmSpec = v1alpha2.VirtualMachineSpec{
			Networks: []v1alpha2.NetworksSpec{},
		}
	})

	It("should return empty list interfaces", func() {
		vmSpec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeMain,
			},
		}

		configs := CreateNetworkSpec(vmSpec)

		Expect(configs).To(HaveLen(0))
	})

	It("should generate correct interface name for Network type", func() {
		vmSpec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "mynet",
			},
		}

		configs := CreateNetworkSpec(vmSpec)

		Expect(configs).To(HaveLen(1))
		Expect(configs[0].Type).To(Equal(v1alpha2.NetworksTypeNetwork))
		Expect(configs[0].Name).To(Equal("mynet"))
		Expect(configs[0].InterfaceName).To(HavePrefix("veth_n"))
	})

	It("should generate correct interface name for ClusterNetwork type", func() {
		vmSpec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeClusterNetwork,
				Name: "clusternet",
			},
		}

		configs := CreateNetworkSpec(vmSpec)

		Expect(configs).To(HaveLen(1))
		Expect(configs[0].Type).To(Equal(v1alpha2.NetworksTypeClusterNetwork))
		Expect(configs[0].Name).To(Equal("clusternet"))
		Expect(configs[0].InterfaceName).To(HavePrefix("veth_cn"))
	})

	It("should generate unique names for different networks with same name and id", func() {
		vmSpec.Networks = []v1alpha2.NetworksSpec{
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "net1",
			},
			{
				Type: v1alpha2.NetworksTypeNetwork,
				Name: "net1",
			},
		}

		configs := CreateNetworkSpec(vmSpec)

		Expect(configs).To(HaveLen(2))
		Expect(configs[0].InterfaceName).NotTo(Equal(configs[1].InterfaceName))
	})
})
