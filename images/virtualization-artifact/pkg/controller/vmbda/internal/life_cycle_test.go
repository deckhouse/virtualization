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

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("LifeCycleHandler helpers", func() {
	DescribeTable("isVirtualMachineMigrating", func(status metav1.ConditionStatus, hasCondition, expected bool) {
		vm := &v1alpha2.VirtualMachine{}
		if hasCondition {
			vm.Status.Conditions = []metav1.Condition{
				{
					Type:   vmcondition.TypeMigrating.String(),
					Status: status,
				},
			}
		}
		Expect(isVirtualMachineMigrating(vm)).To(Equal(expected))
	},
		Entry("migrating condition is true", metav1.ConditionTrue, true, true),
		Entry("migrating condition is false", metav1.ConditionFalse, true, true),
		Entry("migrating condition is unknown", metav1.ConditionUnknown, true, true),
		Entry("migrating condition is absent", metav1.ConditionTrue, false, false),
	)
})
