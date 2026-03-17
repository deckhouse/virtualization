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

package livemigration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("CalculateEffectivePolicy", func() {
	DescribeTable("effective policy and autoConverge value",
		func(
			vmPolicy v1alpha2.LiveMigrationPolicy,
			vmopForce *bool,
			expectedPolicy v1alpha2.LiveMigrationPolicy,
			expectedResult bool,
			expectError bool,
		) {
			vm := v1alpha2.VirtualMachine{}
			if vmPolicy != "" {
				vm.Spec.LiveMigrationPolicy = vmPolicy
			}

			var vmop *v1alpha2.VirtualMachineOperation
			if vmopForce != nil {
				vmop = &v1alpha2.VirtualMachineOperation{
					Spec: v1alpha2.VirtualMachineOperationSpec{
						Force: vmopForce,
					},
				}
			}

			policy, result, err := CalculateEffectivePolicy(vm, vmop)

			if expectError {
				require.Error(GinkgoT(), err)
			} else {
				require.NoError(GinkgoT(), err)
			}

			require.Equal(GinkgoT(), expectedPolicy, policy)
			require.Equal(GinkgoT(), expectedResult, result)
		},

		Entry("PreferForced with no force", v1alpha2.PreferForcedMigrationPolicy, nil, v1alpha2.PreferForcedMigrationPolicy, true, false),
		Entry("PreferForced with force=true", v1alpha2.PreferForcedMigrationPolicy, ptr.To(true), v1alpha2.PreferForcedMigrationPolicy, true, false),
		Entry("PreferForced with force=false", v1alpha2.PreferForcedMigrationPolicy, ptr.To(false), v1alpha2.PreferForcedMigrationPolicy, true, false),

		Entry("PreferSafe with no force", v1alpha2.PreferSafeMigrationPolicy, nil, v1alpha2.PreferSafeMigrationPolicy, false, false),
		Entry("PreferSafe with force=true", v1alpha2.PreferSafeMigrationPolicy, ptr.To(true), v1alpha2.PreferSafeMigrationPolicy, true, false),
		Entry("PreferSafe with force=false", v1alpha2.PreferSafeMigrationPolicy, ptr.To(false), v1alpha2.PreferSafeMigrationPolicy, false, false),

		Entry("AlwaysSafe with no force", v1alpha2.AlwaysSafeMigrationPolicy, nil, v1alpha2.AlwaysSafeMigrationPolicy, false, false),
		Entry("AlwaysSafe with force=true", v1alpha2.AlwaysSafeMigrationPolicy, ptr.To(true), v1alpha2.AlwaysSafeMigrationPolicy, false, true),
		Entry("AlwaysSafe with force=false", v1alpha2.AlwaysSafeMigrationPolicy, ptr.To(false), v1alpha2.AlwaysSafeMigrationPolicy, false, false),

		Entry("AlwaysForced with no force", v1alpha2.AlwaysForcedMigrationPolicy, nil, v1alpha2.AlwaysForcedMigrationPolicy, true, false),
		Entry("AlwaysForced with force=true", v1alpha2.AlwaysForcedMigrationPolicy, ptr.To(true), v1alpha2.AlwaysForcedMigrationPolicy, true, false),
		Entry("AlwaysForced with force=false", v1alpha2.AlwaysForcedMigrationPolicy, ptr.To(false), v1alpha2.AlwaysForcedMigrationPolicy, true, true),

		Entry("No VM policy with no vmop", v1alpha2.LiveMigrationPolicy(""), nil, v1alpha2.PreferSafeMigrationPolicy, false, false),
	)
})

func TestCalculateEffectivePolicy(t *testing.T) {
	gomega.RegisterFailHandler(Fail)
	RunSpecs(t, "CalculateEffectivePolicy Suite")
}
