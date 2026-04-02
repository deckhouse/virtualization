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

var _ = Describe("Network types helpers", func() {
	Describe("HasMainNetworkStatus", func() {
		It("should return true when main network exists", func() {
			statuses := []v1alpha2.NetworksStatus{
				{Type: v1alpha2.NetworksTypeNetwork},
				{Type: v1alpha2.NetworksTypeMain},
			}
			Expect(HasMainNetworkStatus(statuses)).To(BeTrue())
		})

		It("should return false when main network does not exist", func() {
			statuses := []v1alpha2.NetworksStatus{{Type: v1alpha2.NetworksTypeNetwork}}
			Expect(HasMainNetworkStatus(statuses)).To(BeFalse())
		})
	})

	Describe("HasMainNetworkSpec", func() {
		It("should return true when main network exists", func() {
			specs := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeClusterNetwork},
				{Type: v1alpha2.NetworksTypeMain},
			}
			Expect(HasMainNetworkSpec(specs)).To(BeTrue())
		})

		It("should return false when main network does not exist", func() {
			specs := []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork}}
			Expect(HasMainNetworkSpec(specs)).To(BeFalse())
		})
	})

	Describe("InterfaceSpecList.ToString", func() {
		It("should skip main interface in JSON output", func() {
			list := InterfaceSpecList{
				{ID: 1, Type: v1alpha2.NetworksTypeMain, Name: "main", InterfaceName: NameDefaultInterface},
				{ID: 2, Type: v1alpha2.NetworksTypeNetwork, Name: "n1", InterfaceName: "veth_n12345678"},
			}

			out, err := list.ToString()

			Expect(err).ToNot(HaveOccurred())
			Expect(out).To(Equal(`[{"id":2,"type":"Network","name":"n1","ifName":"veth_n12345678"}]`))
		})
	})
})
