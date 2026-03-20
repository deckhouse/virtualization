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

var _ = Describe("Network IDs", func() {
	Describe("EnsureNetworkInterfaceIDs", func() {
		It("should return false for empty list", func() {
			Expect(EnsureNetworkInterfaceIDs(nil)).To(BeFalse())
		})

		It("should assign id=1 to main and next IDs to others", func() {
			networks := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, Name: "main"},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n1"},
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cn1"},
			}

			changed := EnsureNetworkInterfaceIDs(networks)

			Expect(changed).To(BeTrue())
			Expect(*networks[0].ID).To(Equal(1))
			Expect(*networks[1].ID).To(Equal(2))
			Expect(*networks[2].ID).To(Equal(3))
		})

		It("should preserve existing IDs and assign gaps", func() {
			idMain := 1
			idNet := 5
			networks := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, Name: "main", ID: &idMain},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n1", ID: &idNet},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n2"},
			}

			changed := EnsureNetworkInterfaceIDs(networks)

			Expect(changed).To(BeTrue())
			Expect(*networks[0].ID).To(Equal(1))
			Expect(*networks[1].ID).To(Equal(5))
			Expect(*networks[2].ID).To(Equal(2))
		})

		It("should return false when nothing to assign", func() {
			idMain := 1
			idNet := 2
			networks := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: &idMain},
				{Type: v1alpha2.NetworksTypeNetwork, ID: &idNet},
			}

			changed := EnsureNetworkInterfaceIDs(networks)
			Expect(changed).To(BeFalse())
		})
	})
})
