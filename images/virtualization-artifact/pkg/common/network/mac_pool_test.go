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

package network

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("MacAddressPool", func() {
	newVMMAC := func(address string) *v1alpha2.VirtualMachineMACAddress {
		return &v1alpha2.VirtualMachineMACAddress{
			Status: v1alpha2.VirtualMachineMACAddressStatus{Address: address},
		}
	}

	It("should use reserved MAC for known network and free MACs for new ones", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a"},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-b"},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-c"},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a", MAC: "00:1A:2B:3C:4D:5E"},
				},
			},
		}

		pool := NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{
			newVMMAC("00:1A:2B:3C:4D:5E"),
			newVMMAC("00:1A:2B:3C:4D:5F"),
			newVMMAC("00:1A:2B:3C:4D:6A"),
		}, true)

		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-a")).To(Equal("00:1A:2B:3C:4D:5E"))
		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-b")).To(Equal("00:1A:2B:3C:4D:5F"))
		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-c")).To(Equal("00:1A:2B:3C:4D:6A"))
	})

	It("should bind free MACs to networks regardless of the listing order and of the rendered subset", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeMain},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a"},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-b"},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-c"},
				},
			},
		}
		expected := []string{"00:1A:2B:3C:4D:5E", "00:1A:2B:3C:4D:5F", "00:1A:2B:3C:4D:6A"}

		pool := NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{newVMMAC(expected[2]), newVMMAC(expected[0]), newVMMAC(expected[1])}, true)
		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-c")).To(Equal(expected[2]))
		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-a")).To(Equal(expected[0]))

		pool = NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{newVMMAC(expected[1]), newVMMAC(expected[2]), newVMMAC(expected[0])}, true)
		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-b")).To(Equal(expected[1]))
	})

	It("should tell a Network from a ClusterNetwork of the same name", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net"},
					{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net"},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net", MAC: "00:1A:2B:3C:4D:6A"},
					{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "net", MAC: "00:1A:2B:3C:4D:5E"},
				},
			},
		}

		pool := NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{
			newVMMAC("00:1A:2B:3C:4D:5E"),
			newVMMAC("00:1A:2B:3C:4D:6A"),
		}, true)

		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net")).To(Equal("00:1A:2B:3C:4D:6A"))
		Expect(pool.MACFor(v1alpha2.NetworksTypeClusterNetwork, "net")).To(Equal("00:1A:2B:3C:4D:5E"))
	})

	It("should bind a MAC address pinned in the spec to its network", func() {
		pinned := func(name, address string) *v1alpha2.VirtualMachineMACAddress {
			vmmac := newVMMAC(address)
			vmmac.Name = name
			return vmmac
		}
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a", VirtualMachineMACAddressName: "mac-a"},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-b", VirtualMachineMACAddressName: "mac-b"},
				},
			},
		}

		pool := NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{
			pinned("mac-a", "00:1A:2B:3C:4D:6A"),
			pinned("mac-b", "00:1A:2B:3C:4D:5E"),
		}, true)

		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-a")).To(Equal("00:1A:2B:3C:4D:6A"))
		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-b")).To(Equal("00:1A:2B:3C:4D:5E"))
	})

	It("should keep the MAC address of the status over a pin changed on a running machine", func() {
		pinned := newVMMAC("00:1A:2B:3C:4D:6A")
		pinned.Name = "mac-new"
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a", VirtualMachineMACAddressName: "mac-new"},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a", MAC: "00:1A:2B:3C:4D:5E"},
				},
			},
		}

		pool := NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{pinned, newVMMAC("00:1A:2B:3C:4D:5E")}, true)

		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-a")).To(Equal("00:1A:2B:3C:4D:5E"))
	})

	It("should let a pin win over the status for a machine without a KVVM", func() {
		pinned := newVMMAC("00:1A:2B:3C:4D:6A")
		pinned.Name = "mac-restored"
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a", VirtualMachineMACAddressName: "mac-restored"},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-b"},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Networks: []v1alpha2.NetworksStatus{
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a", MAC: "00:1A:2B:3C:4D:5E"},
				},
			},
		}

		pool := NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{pinned, newVMMAC("00:1A:2B:3C:4D:5E")}, false)

		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-a")).To(Equal("00:1A:2B:3C:4D:6A"))
		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-b")).To(Equal("00:1A:2B:3C:4D:5E"))
	})

	It("should return empty MAC when pool is exhausted", func() {
		vm := &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				Networks: []v1alpha2.NetworksSpec{
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-a"},
					{Type: v1alpha2.NetworksTypeNetwork, Name: "net-b"},
				},
			},
		}
		pool := NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{
			newVMMAC("00:1A:2B:3C:4D:5E"),
		}, true)

		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-a")).To(Equal("00:1A:2B:3C:4D:5E"))
		Expect(pool.MACFor(v1alpha2.NetworksTypeNetwork, "net-b")).To(Equal(""))
	})
})
