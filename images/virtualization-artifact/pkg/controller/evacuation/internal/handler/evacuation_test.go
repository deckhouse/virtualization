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
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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

	newNode := func(drained bool) *corev1.Node {
		node := &corev1.Node{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Node",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
		}
		if drained {
			node.Spec = corev1.NodeSpec{
				Taints: []corev1.Taint{
					{
						Key:    "kubevirt.io/drain",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
				Unschedulable: true,
			}
		}
		return node
	}

	newVM := func(needEvict bool) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(vmName, vmNamespace)
		vm.Status.Node = nodeName
		if needEvict {
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:   vmcondition.TypeNeedEvict.String(),
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
		func(node *corev1.Node, vm *v1alpha2.VirtualMachine, vmop *v1alpha2.VirtualMachineOperation, shouldEvict bool) {
			fakeClient = setupEnvironment(node, vm, vmop)

			newNode := &corev1.Node{}
			err := fakeClient.Get(ctx, client.ObjectKey{Name: nodeName}, newNode)
			Expect(err).NotTo(HaveOccurred())

			h := NewEvacuationHandler(fakeClient)
			_, err = h.Handle(ctx, newNode)
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
		Entry("Should create vmop because Node drained", newNode(true), newVM(false), nil, true),
		Entry("Should create vmop because VM evicted", newNode(false), newVM(true), nil, true),
		Entry("Should do nothing", newNode(false), newVM(false), nil, false),
		Entry("Should do nothing because VM already migrating", newNode(true), newVM(true), newVMOP(v1alpha2.VMOPPhaseInProgress), false),
		Entry("Should create vmop because Node drained but old vmop finished", newNode(true), newVM(false), newVMOP(v1alpha2.VMOPPhaseFailed), true),
		Entry("Should create vmop because VM evicted but old vmop finished", newNode(true), newVM(false), newVMOP(v1alpha2.VMOPPhaseCompleted), true),
	)
})
