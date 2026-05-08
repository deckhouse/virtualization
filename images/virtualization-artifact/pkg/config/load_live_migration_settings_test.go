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

package config

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SystemMigrationPolicyOverride", func() {
	BeforeEach(func() {
		ResetSystemMigrationPolicyOverride()
	})

	AfterEach(func() {
		ResetSystemMigrationPolicyOverride()
	})

	DescribeTable("accepts valid values",
		func(policy v1alpha2.LiveMigrationPolicy) {
			ok := SetSystemMigrationPolicyOverride(string(policy))
			Expect(ok).To(BeTrue())

			actual, exists := GetSystemMigrationPolicyOverride()
			Expect(exists).To(BeTrue())
			Expect(actual).To(Equal(policy))
		},
		Entry("Manual", v1alpha2.ManualMigrationPolicy),
		Entry("Never", v1alpha2.NeverMigrationPolicy),
		Entry("AlwaysSafe", v1alpha2.AlwaysSafeMigrationPolicy),
		Entry("PreferSafe", v1alpha2.PreferSafeMigrationPolicy),
		Entry("AlwaysForced", v1alpha2.AlwaysForcedMigrationPolicy),
		Entry("PreferForced", v1alpha2.PreferForcedMigrationPolicy),
	)

	It("rejects invalid value", func() {
		ok := SetSystemMigrationPolicyOverride("invalid")
		Expect(ok).To(BeFalse())

		actual, exists := GetSystemMigrationPolicyOverride()
		Expect(exists).To(BeFalse())
		Expect(actual).To(Equal(v1alpha2.LiveMigrationPolicy("")))
	})

	It("reset clears override", func() {
		ok := SetSystemMigrationPolicyOverride(string(v1alpha2.PreferSafeMigrationPolicy))
		Expect(ok).To(BeTrue())

		ResetSystemMigrationPolicyOverride()

		actual, exists := GetSystemMigrationPolicyOverride()
		Expect(exists).To(BeFalse())
		Expect(actual).To(Equal(v1alpha2.LiveMigrationPolicy("")))
	})
})

func TestSystemMigrationPolicyOverride(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SystemMigrationPolicyOverride Suite")
}
