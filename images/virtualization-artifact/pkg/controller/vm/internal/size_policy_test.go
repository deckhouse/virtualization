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
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("SizePolicyHandler", func() {
	const (
		name        = "vm-size"
		namespace   = "default"
		vmClassName = "vmclass"
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

	newVM := func(vmClassName string) *virtv2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		if vmClassName != "" {
			vm.Spec.VirtualMachineClassName = vmClassName
		}

		return vm
	}

	reconcile := func() {
		h := NewSizePolicyHandler()
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Condition presence and absence scenarios", func() {
		It("Should add condition if it was absent and size policy does not match", func() {
			vm := newVM("")
			fakeClient, resource, vmState = setupEnvironment(vm)
			reconcile()

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())
			cond, exists := conditions.GetCondition(vmcondition.TypeSizingPolicyMatched, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("Should remove condition if it was present and size policy matches now", func() {
			vm := newVM(vmClassName)
			vm.Status.Conditions = []metav1.Condition{
				{
					Type:   vmcondition.TypeSizingPolicyMatched.String(),
					Status: metav1.ConditionFalse,
				},
			}

			vmClass := &virtv2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmClassName,
				},
			}
			fakeClient, resource, vmState = setupEnvironment(vm, vmClass)
			reconcile()

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())
			_, exists := conditions.GetCondition(vmcondition.TypeSizingPolicyMatched, newVM.Status.Conditions)
			Expect(exists).To(BeFalse())
		})

		It("Should not add condition if it was absent and size policy matches", func() {
			vm := newVM(vmClassName)

			vmClass := &virtv2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmClassName,
				},
			}
			fakeClient, resource, vmState = setupEnvironment(vm, vmClass)
			reconcile()

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())
			_, exists := conditions.GetCondition(vmcondition.TypeSizingPolicyMatched, newVM.Status.Conditions)
			Expect(exists).To(BeFalse())
		})
	})
})
