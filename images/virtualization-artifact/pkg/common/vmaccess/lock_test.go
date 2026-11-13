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

package vmaccess

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

var _ = Describe("Lock", func() {
	It("is written, read back and removed without touching other annotations", func() {
		obj := &metav1.ObjectMeta{Annotations: map[string]string{"other": "kept"}}
		want := &Lock{ClaimUID: "claim", VMIUID: "vmi"}

		Expect(WriteLock(obj, want)).To(Succeed())
		Expect(ReadLock(obj)).To(Equal(want))

		Expect(WriteLock(obj, nil)).To(Succeed())
		Expect(ReadLock(obj)).To(BeNil())
		Expect(obj.Annotations).To(HaveKeyWithValue("other", "kept"))
	})

	DescribeTable("rejects a malformed record",
		func(raw string) {
			obj := &metav1.ObjectMeta{Annotations: map[string]string{annotations.AnnVMAccessLock: raw}}
			_, err := ReadLock(obj)
			Expect(err).To(MatchError(ErrMalformedLock))
		},
		Entry("not json", "claim"),
		Entry("no claim", `{"vmiUID":"vmi"}`),
	)
})

var _ = DescribeTable("CheckCall",
	func(lock *Lock, claimUID string, allowed bool) {
		err := CheckCall(lock, claimUID)
		if allowed {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
	},
	Entry("no lock, no claim", nil, "", true),
	Entry("no lock, some claim", nil, "other", true),
	Entry("lock, no claim", &Lock{ClaimUID: "holder"}, "", false),
	Entry("lock, another claim", &Lock{ClaimUID: "holder"}, "other", false),
	Entry("lock, the holding claim", &Lock{ClaimUID: "holder"}, "holder", true),
)
