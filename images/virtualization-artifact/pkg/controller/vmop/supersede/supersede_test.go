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

package supersede

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestSupersede(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Supersede Suite")
}

var _ = Describe("CanSupersede", func() {
	DescribeTable("allowed combinations",
		func(oldType v1alpha2.VMOPType, oldForce bool, newType v1alpha2.VMOPType, newForce bool) {
			Expect(CanSupersede(vmop(oldType, oldForce), vmop(newType, newForce))).To(BeTrue())
		},
		Entry("start by stop", v1alpha2.VMOPTypeStart, false, v1alpha2.VMOPTypeStop, false),
		Entry("start by force stop", v1alpha2.VMOPTypeStart, false, v1alpha2.VMOPTypeStop, true),
		Entry("start by restart", v1alpha2.VMOPTypeStart, false, v1alpha2.VMOPTypeRestart, false),
		Entry("start by force restart", v1alpha2.VMOPTypeStart, false, v1alpha2.VMOPTypeRestart, true),
		Entry("stop by force stop", v1alpha2.VMOPTypeStop, false, v1alpha2.VMOPTypeStop, true),
		Entry("migrate by stop", v1alpha2.VMOPTypeMigrate, false, v1alpha2.VMOPTypeStop, false),
		Entry("migrate by force stop", v1alpha2.VMOPTypeMigrate, false, v1alpha2.VMOPTypeStop, true),
		Entry("evict by stop", v1alpha2.VMOPTypeEvict, false, v1alpha2.VMOPTypeStop, false),
		Entry("restart by stop", v1alpha2.VMOPTypeRestart, false, v1alpha2.VMOPTypeStop, false),
		Entry("force restart by force stop", v1alpha2.VMOPTypeRestart, true, v1alpha2.VMOPTypeStop, true),
	)

	DescribeTable("forbidden combinations",
		func(oldType v1alpha2.VMOPType, oldForce bool, newType v1alpha2.VMOPType, newForce bool) {
			Expect(CanSupersede(vmop(oldType, oldForce), vmop(newType, newForce))).To(BeFalse())
		},
		Entry("start by start", v1alpha2.VMOPTypeStart, false, v1alpha2.VMOPTypeStart, false),
		Entry("stop by stop", v1alpha2.VMOPTypeStop, false, v1alpha2.VMOPTypeStop, false),
		Entry("force stop by force stop", v1alpha2.VMOPTypeStop, true, v1alpha2.VMOPTypeStop, true),
		Entry("stop by start", v1alpha2.VMOPTypeStop, false, v1alpha2.VMOPTypeStart, false),
		Entry("migrate by migrate", v1alpha2.VMOPTypeMigrate, false, v1alpha2.VMOPTypeMigrate, false),
		Entry("restore by stop", v1alpha2.VMOPTypeRestore, false, v1alpha2.VMOPTypeStop, true),
		Entry("clone by stop", v1alpha2.VMOPTypeClone, false, v1alpha2.VMOPTypeStop, true),
	)

	It("denies operations for different virtual machines", func() {
		oldVMOP := vmop(v1alpha2.VMOPTypeStart, false)
		newVMOP := vmop(v1alpha2.VMOPTypeStop, false)
		newVMOP.Spec.VirtualMachine = "another-vm"

		Expect(CanSupersede(oldVMOP, newVMOP)).To(BeFalse())
	})
})

func vmop(vmopType v1alpha2.VMOPType, force bool) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           vmopType,
			VirtualMachine: "test-vm",
			Force:          ptr.To(force),
		},
	}
}
