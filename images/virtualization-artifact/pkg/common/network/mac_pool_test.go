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
		})

		Expect(pool.Assign("net-a")).To(Equal("00:1A:2B:3C:4D:5E"))
		Expect(pool.Assign("net-b")).To(Equal("00:1A:2B:3C:4D:5F"))
		Expect(pool.Assign("net-c")).To(Equal("00:1A:2B:3C:4D:6A"))
	})

	It("should return empty MAC when pool is exhausted", func() {
		vm := &v1alpha2.VirtualMachine{}
		pool := NewMacAddressPool(vm, []*v1alpha2.VirtualMachineMACAddress{
			newVMMAC("00:1A:2B:3C:4D:5E"),
		})

		Expect(pool.Assign("net-a")).To(Equal("00:1A:2B:3C:4D:5E"))
		Expect(pool.Assign("net-b")).To(Equal(""))
	})
})
