/*
Copyright 2025 Flant JSC

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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("TestEvictHandler", func() {
	const (
		name      = "vm-evict"
		namespace = "default"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		resource   *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
	)

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
	})

	newVM := func(withCond bool) *virtv2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		if withCond {
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:    vmcondition.TypeNeedsEvict.String(),
				Status:  metav1.ConditionTrue,
				Reason:  vmcondition.ReasonNeedsEvict.String(),
				Message: "Some message",
			})
		}
		return vm
	}

	newKVVMI := func(evacuationNodeName string) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.EvacuationNodeName = evacuationNodeName
		return kvvmi
	}

	reconcile := func() {
		h := NewEvictHandler()
		_, err := h.Handle(testutil.ContextBackgroundWithNoOpLogger(), vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	DescribeTable("Condition NeedEvict should be in expected state",
		func(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, condShouldExists bool, expectedStatus metav1.ConditionStatus, expectedReason vmcondition.Reason) {
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)
			reconcile()

			newVM := &virtv2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			needEvict, exists := conditions.GetCondition(vmcondition.TypeNeedsEvict, newVM.Status.Conditions)
			if condShouldExists {
				Expect(exists).To(BeTrue())
				Expect(needEvict.Status).To(Equal(expectedStatus))
				Expect(needEvict.Reason).To(Equal(expectedReason.String()))
			} else {
				Expect(exists).To(BeFalse())
			}
		},
		Entry("Should add NeedEvict condition when KVVM has evacuation node", newVM(false), newKVVMI("node1"), true, metav1.ConditionTrue, vmcondition.ReasonNeedsEvict),
		Entry("Should remove NeedEvict condition when KVVM has no evacuation node", newVM(true), newKVVMI(""), false, metav1.ConditionStatus(""), vmcondition.Reason("")),
		Entry("Should not change NeedEvict condition when condition is present and KVVM has evacuation node", newVM(true), newKVVMI("node1"), true, metav1.ConditionTrue, vmcondition.ReasonNeedsEvict),
	)
})
