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

package v1alpha2

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CoreFractionValue JSON", func() {
	DescribeTable("decodes both the v1alpha2 number and the v1alpha3 percentage string",
		func(raw string, expected []CoreFractionValue) {
			var policy SizingPolicy
			Expect(json.Unmarshal([]byte(`{"coreFractions":`+raw+`}`), &policy)).To(Succeed())
			Expect(policy.CoreFractions).To(Equal(expected))
		},
		Entry("numbers", `[5, 100]`, []CoreFractionValue{5, 100}),
		Entry("percentage strings stored by a v1alpha3 client", `["5%", "100%"]`, []CoreFractionValue{5, 100}),
		Entry("mixed", `[5, "10%"]`, []CoreFractionValue{5, 10}),
		Entry("null", `null`, nil),
	)

	It("rejects a string that is not a percentage", func() {
		var policy SizingPolicy
		Expect(json.Unmarshal([]byte(`{"coreFractions":["Auto"]}`), &policy)).To(MatchError(ContainSubstring(`"Auto"`)))
	})

	It("still encodes as a number", func() {
		out, err := json.Marshal([]CoreFractionValue{5, 100})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(Equal(`[5,100]`))
	})
})
