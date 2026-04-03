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

		It("should skip already reserved IDs even if they appear later in list", func() {
			idMain := 1
			idMax := 16383
			idNet := 2
			networks := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: &idMain},
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cnet-521"},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "net-501", ID: &idMax},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "net-502", ID: &idNet},
			}

			changed := EnsureNetworkInterfaceIDs(networks)

			Expect(changed).To(BeTrue())
			Expect(*networks[0].ID).To(Equal(1))
			Expect(*networks[1].ID).To(Equal(3))
			Expect(*networks[2].ID).To(Equal(16383))
			Expect(*networks[3].ID).To(Equal(2))
		})

		It("should assign minimal free IDs for multiple missing networks", func() {
			idMain := 1
			id4 := 4
			id7 := 7
			networks := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, ID: &idMain},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n-missing-1"},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n-4", ID: &id4},
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cn-missing-1"},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n-7", ID: &id7},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n-missing-2"},
			}

			changed := EnsureNetworkInterfaceIDs(networks)

			Expect(changed).To(BeTrue())
			Expect(*networks[1].ID).To(Equal(2))
			Expect(*networks[3].ID).To(Equal(3))
			Expect(*networks[5].ID).To(Equal(5))
		})

		It("should assign generic IDs starting from 2 when there is no main", func() {
			id4 := 4
			networks := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n1"},
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cn1"},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n4", ID: &id4},
			}

			changed := EnsureNetworkInterfaceIDs(networks)

			Expect(changed).To(BeTrue())
			Expect(*networks[0].ID).To(Equal(2))
			Expect(*networks[1].ID).To(Equal(3))
			Expect(*networks[2].ID).To(Equal(4))
		})

		It("should stop assignment when generic ID space is exhausted", func() {
			idMain := 1
			networks := []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, ID: &idMain}}
			for id := StartGenericID; id <= MaxID; id++ {
				v := id
				networks = append(networks, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeNetwork, ID: &v})
			}
			networks = append(networks, v1alpha2.NetworksSpec{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "overflow"})

			changed := EnsureNetworkInterfaceIDs(networks)

			Expect(changed).To(BeFalse())
			Expect(networks[len(networks)-1].ID).To(BeNil())
		})

		It("should keep duplicate id=1 when main has no ID but id=1 is already used", func() {
			id1 := 1
			networks := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n-with-1", ID: &id1},
				{Type: v1alpha2.NetworksTypeMain, Name: "main"},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n-missing"},
			}

			changed := EnsureNetworkInterfaceIDs(networks)

			Expect(changed).To(BeTrue())
			Expect(*networks[0].ID).To(Equal(1))
			Expect(*networks[1].ID).To(Equal(1))
			Expect(*networks[2].ID).To(Equal(2))
		})

		It("should preserve explicit invalid IDs and assign only missing ones", func() {
			idZero := 0
			idNegative := -5
			idTooHigh := 20000
			networks := []v1alpha2.NetworksSpec{
				{Type: v1alpha2.NetworksTypeMain, Name: "main", ID: &idZero},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n-neg", ID: &idNegative},
				{Type: v1alpha2.NetworksTypeNetwork, Name: "n-high", ID: &idTooHigh},
				{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cn-missing"},
			}

			changed := EnsureNetworkInterfaceIDs(networks)

			Expect(changed).To(BeTrue())
			Expect(*networks[0].ID).To(Equal(0))
			Expect(*networks[1].ID).To(Equal(-5))
			Expect(*networks[2].ID).To(Equal(20000))
			Expect(*networks[3].ID).To(Equal(2))
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
