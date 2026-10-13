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

package watcher

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
)

var _ = DescribeTable("ipAddressChangeAffectsVM",
	func(change func(*sdnapi.IPAddress), expected bool) {
		base := func() *sdnapi.IPAddress {
			return &sdnapi.IPAddress{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"}},
				Status: sdnapi.IPAddressStatus{
					Address:    "10.66.10.14",
					UsedByPods: []string{"virt-launcher-vm-abcde"},
					Conditions: []metav1.Condition{{Type: sdnapi.ConditionAllocated, Status: metav1.ConditionTrue}},
				},
			}
		}
		oldIPA, newIPA := base(), base()
		change(newIPA)

		Expect(ipAddressChangeAffectsVM(oldIPA, newIPA)).To(Equal(expected))
	},
	Entry("nothing the VM reads", func(ipa *sdnapi.IPAddress) { ipa.SetAnnotations(map[string]string{"a": "b"}) }, false),
	Entry("allocation", func(ipa *sdnapi.IPAddress) { ipa.Status.Conditions[0].Status = metav1.ConditionFalse }, true),
	Entry("address", func(ipa *sdnapi.IPAddress) { ipa.Status.Address = "10.66.10.15" }, true),
	Entry("owner label", func(ipa *sdnapi.IPAddress) { ipa.SetLabels(nil) }, true),
	Entry("deletion starts", func(ipa *sdnapi.IPAddress) { now := metav1.Now(); ipa.SetDeletionTimestamp(&now) }, true),
	Entry("the last pod leaves", func(ipa *sdnapi.IPAddress) { ipa.Status.UsedByPods = nil }, true),
	Entry("another pod joins", func(ipa *sdnapi.IPAddress) {
		ipa.Status.UsedByPods = append(ipa.Status.UsedByPods, "virt-launcher-vm-fghij")
	}, false),
)
