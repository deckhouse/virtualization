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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
		resource   *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
	)

	AfterEach(func() {
		fakeClient = nil
		resource = nil
		vmState = nil
	})

	newVM := func(withCond bool) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		if withCond {
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:    vmcondition.TypeEvictionRequired.String(),
				Status:  metav1.ConditionTrue,
				Reason:  vmcondition.ReasonEvictionRequired.String(),
				Message: "Some message",
			})
		}
		return vm
	}

	newKVVMI := func(evacuationNodeName string, phase virtv1.VirtualMachineInstancePhase) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.EvacuationNodeName = evacuationNodeName
		kvvmi.Status.Phase = phase
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
		func(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, condShouldExists bool, expectedStatus metav1.ConditionStatus, expectedReason vmcondition.EvictionRequiredReason) {
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			needEvict, exists := conditions.GetCondition(vmcondition.TypeEvictionRequired, newVM.Status.Conditions)
			if condShouldExists {
				Expect(exists).To(BeTrue())
				Expect(needEvict.Status).To(Equal(expectedStatus))
				Expect(needEvict.Reason).To(Equal(expectedReason.String()))
			} else {
				Expect(exists).To(BeFalse())
			}
		},
		Entry("Should add NeedEvict condition when KVVM has evacuation node", newVM(false), newKVVMI("node1", virtv1.Running), true, metav1.ConditionTrue, vmcondition.ReasonEvictionRequired),
		Entry("Should remove NeedEvict condition when KVVM has no evacuation node", newVM(true), newKVVMI("", virtv1.Running), false, metav1.ConditionStatus(""), vmcondition.EvictionRequiredReason("")),
		Entry("Should not change NeedEvict condition when condition is present and KVVM has evacuation node", newVM(true), newKVVMI("node1", virtv1.Running), true, metav1.ConditionTrue, vmcondition.ReasonEvictionRequired),
		Entry("Shoiuld remove NeedEvict condition when KVVM is not running", newVM(true), newKVVMI("", virtv1.Failed), false, metav1.ConditionStatus(""), vmcondition.EvictionRequiredReason("")),
	)
})
