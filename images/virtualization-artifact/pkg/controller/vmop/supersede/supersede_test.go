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
	"fmt"
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
	Describe("matrix combinations", func() {
		for _, entry := range supersedeMatrixEntries() {
			It(entry.name, func() {
				Expect(CanSupersede(vmop(entry.oldType, entry.oldForce), vmop(entry.newType, entry.newForce))).To(Equal(entry.expected))
			})
		}
	})

	It("denies operations for different virtual machines", func() {
		oldVMOP := vmop(v1alpha2.VMOPTypeStart, false)
		newVMOP := vmop(v1alpha2.VMOPTypeStop, false)
		newVMOP.Spec.VirtualMachine = "another-vm"

		Expect(CanSupersede(oldVMOP, newVMOP)).To(BeFalse())
	})

	DescribeTable("denies nil operations",
		func(oldVMOP, newVMOP *v1alpha2.VirtualMachineOperation) {
			Expect(CanSupersede(oldVMOP, newVMOP)).To(BeFalse())
		},
		Entry("nil old operation", nil, vmop(v1alpha2.VMOPTypeStop, false)),
		Entry("nil new operation", vmop(v1alpha2.VMOPTypeStart, false), nil),
		Entry("nil operations", nil, nil),
	)
})

type supersedeMatrixEntry struct {
	name     string
	oldType  v1alpha2.VMOPType
	oldForce bool
	newType  v1alpha2.VMOPType
	newForce bool
	expected bool
}

func supersedeMatrixEntries() []supersedeMatrixEntry {
	vmopTypes := []v1alpha2.VMOPType{
		v1alpha2.VMOPTypeStart,
		v1alpha2.VMOPTypeStop,
		v1alpha2.VMOPTypeMigrate,
		v1alpha2.VMOPTypeEvict,
		v1alpha2.VMOPTypeRestart,
		v1alpha2.VMOPTypeRestore,
		v1alpha2.VMOPTypeClone,
	}
	forces := []bool{false, true}

	var entries []supersedeMatrixEntry
	for _, oldType := range vmopTypes {
		for _, oldForce := range forces {
			for _, newType := range vmopTypes {
				for _, newForce := range forces {
					entries = append(entries, supersedeMatrixEntry{
						name:     fmt.Sprintf("%s force=%t by %s force=%t", oldType, oldForce, newType, newForce),
						oldType:  oldType,
						oldForce: oldForce,
						newType:  newType,
						newForce: newForce,
						expected: expectedCanSupersede(oldType, oldForce, newType, newForce),
					})
				}
			}
		}
	}

	return entries
}

func expectedCanSupersede(oldType v1alpha2.VMOPType, oldForce bool, newType v1alpha2.VMOPType, newForce bool) bool {
	switch oldType {
	case v1alpha2.VMOPTypeStart:
		return newType == v1alpha2.VMOPTypeStop
	case v1alpha2.VMOPTypeStop:
		if oldForce {
			return false
		}
		return newType == v1alpha2.VMOPTypeStop && newForce ||
			newType == v1alpha2.VMOPTypeRestart && newForce
	case v1alpha2.VMOPTypeMigrate, v1alpha2.VMOPTypeEvict:
		return newType == v1alpha2.VMOPTypeStop || newType == v1alpha2.VMOPTypeRestart
	case v1alpha2.VMOPTypeRestart:
		if oldForce {
			return false
		}
		return newType == v1alpha2.VMOPTypeStop && newForce ||
			newType == v1alpha2.VMOPTypeRestart && newForce
	default:
		return false
	}
}

func vmop(vmopType v1alpha2.VMOPType, force bool) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           vmopType,
			VirtualMachine: "test-vm",
			Force:          ptr.To(force),
		},
	}
}
