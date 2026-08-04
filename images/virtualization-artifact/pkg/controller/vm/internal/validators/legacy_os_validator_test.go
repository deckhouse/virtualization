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

package validators

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("LegacyOSValidator (VM)", func() {
	makeVM := func(osType v1alpha2.OsType, paravirt *bool) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			Spec: v1alpha2.VirtualMachineSpec{
				OsType:                   osType,
				EnableParavirtualization: paravirt,
			},
		}
	}

	DescribeTable("warns only about a Legacy VM left with paravirtualization on",
		func(osType v1alpha2.OsType, paravirt *bool, expectWarning bool) {
			v := NewLegacyOSValidator()

			warnings, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), makeVM(osType, paravirt))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(warnings).Should(HaveLen(map[bool]int{true: 1, false: 0}[expectWarning]))

			warnings, err = v.ValidateUpdate(testutil.ContextBackgroundWithNoOpLogger(), makeVM(osType, paravirt), makeVM(osType, paravirt))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(warnings).Should(HaveLen(map[bool]int{true: 1, false: 0}[expectWarning]))
		},
		// enableParavirtualization is unset only in tests: the CRD defaults it to true,
		// which is exactly the case the warning exists for.
		Entry("Legacy, paravirtualization defaulted", v1alpha2.LegacyOs, nil, true),
		Entry("Legacy, paravirtualization on", v1alpha2.LegacyOs, ptr.To(true), true),
		Entry("Legacy, paravirtualization off", v1alpha2.LegacyOs, ptr.To(false), false),
		Entry("Generic, paravirtualization on", v1alpha2.GenericOs, ptr.To(true), false),
		Entry("Windows, paravirtualization on", v1alpha2.Windows, ptr.To(true), false),
	)
})
