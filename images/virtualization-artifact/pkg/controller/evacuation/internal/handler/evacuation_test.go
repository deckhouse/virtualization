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

package handler

import (
	"cmp"
	"context"
	"errors"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("TestEvacuationHandler", func() {
	const (
		nodeName    = "worker-0"
		vmName      = "vm-evacuate"
		vmNamespace = "default"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
	)

	AfterEach(func() {
		fakeClient = nil
	})

	newVM := func(needEvict bool) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(vmName, vmNamespace)
		vm.Status.Node = nodeName
		if needEvict {
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:   vmcondition.TypeEvictionRequired.String(),
				Status: metav1.ConditionTrue,
			})
		}
		return vm
	}

	newVMOP := func(phase v1alpha2.VMOPPhase) *v1alpha2.VirtualMachineOperation {
		vmop := newEvacuationVMOP(vmName, vmNamespace)
		vmop.Status.Phase = phase
		return vmop
	}

	DescribeTable("Trigger Evacuate vm",
		func(vm *v1alpha2.VirtualMachine, vmop *v1alpha2.VirtualMachineOperation, shouldEvict bool) {
			fakeClient = setupEnvironment(vm, vmop)

			h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
				return nil
			}})
			_, err := h.Handle(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			vmops := v1alpha2.VirtualMachineOperationList{}
			err = fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))
			Expect(err).NotTo(HaveOccurred())

			slices.SortFunc(vmops.Items, func(a, b v1alpha2.VirtualMachineOperation) int {
				return cmp.Compare(a.CreationTimestamp.UnixNano(), b.CreationTimestamp.UnixNano())
			})

			vmopCount := 0
			if vmop != nil {
				vmopCount++
			}

			if shouldEvict {
				Expect(len(vmops.Items)).To(Equal(vmopCount + 1))

				vmop := vmops.Items[len(vmops.Items)-1]
				Expect(vmop.Spec.Type).To(Equal(v1alpha2.VMOPTypeEvict))
				_, exists := vmop.GetAnnotations()[annotations.AnnVMOPEvacuation]
				Expect(exists).To(Equal(true))
			} else {
				Expect(len(vmops.Items)).To(Equal(vmopCount))
			}
		},
		Entry("Should create vmop because VM evicted", newVM(true), nil, true),
		Entry("Should do nothing", newVM(false), nil, false),
		Entry("Should do nothing because VM already migrating", newVM(true), newVMOP(v1alpha2.VMOPPhaseInProgress), false),
		Entry("Should create vmop because VM evicted but old vmop finished", newVM(true), newVMOP(v1alpha2.VMOPPhaseCompleted), true),
	)

	Context("Cancel Evacuation", func() {
		It("Should cancel evacuation", func() {
			expectErr := errors.New("expectErr")
			canceler := &EvacuateCancelerMock{
				CancelFunc: func(_ context.Context, _, _ string) error {
					return expectErr
				},
			}

			vmop := newVMOP(v1alpha2.VMOPPhaseInProgress)
			vmop.Name = "evacuation-12345"

			fakeClient = setupEnvironment(newVM(true), vmop)
			h := NewEvacuationHandler(fakeClient, canceler)

			err := fakeClient.Delete(ctx, vmop)
			Expect(err).NotTo(HaveOccurred())

			newVM := &v1alpha2.VirtualMachine{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: vmName, Namespace: vmNamespace}, newVM)
			Expect(err).NotTo(HaveOccurred())

			_, err = h.Handle(testutil.ContextBackgroundWithNoOpLogger(), newVM)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expectErr))
		})
	})
})
