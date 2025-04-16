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

var _ = Describe("SnapshottingHandler", func() {
	const (
		name      = "vm-snapshot"
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

	newVM := func() *virtv2.VirtualMachine {
		return vmbuilder.NewEmpty(name, namespace)
	}

	newVMSnapshot := func(vmName string, phase virtv2.VirtualMachineSnapshotPhase) *virtv2.VirtualMachineSnapshot {
		return &virtv2.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName + "-snapshot",
				Namespace: namespace,
			},
			Spec: virtv2.VirtualMachineSnapshotSpec{
				VirtualMachineName: vmName,
			},
			Status: virtv2.VirtualMachineSnapshotStatus{
				Phase: phase,
			},
		}
	}

	reconcile := func() {
		h := NewSnapshottingHandler(fakeClient)
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Condition presence and absence scenarios", func() {
		It("Should add condition if snapshot is in progress", func() {
			vm := newVM()
			snapshot := newVMSnapshot(vm.Name, virtv2.VirtualMachineSnapshotPhaseInProgress)
			fakeClient, resource, vmState = setupEnvironment(vm, snapshot)

			reconcile()

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeSnapshotting, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("Should not add condition if snapshot is ready", func() {
			vm := newVM()
			snapshot := newVMSnapshot(vm.Name, virtv2.VirtualMachineSnapshotPhaseReady)
			fakeClient, resource, vmState = setupEnvironment(vm, snapshot)
			reconcile()

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			_, exists := conditions.GetCondition(vmcondition.TypeSnapshotting, newVM.Status.Conditions)
			Expect(exists).To(BeFalse())
		})

		It("Should not add condition if no snapshots exist", func() {
			vm := newVM()
			fakeClient, resource, vmState = setupEnvironment(vm)
			reconcile()

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			_, exists := conditions.GetCondition(vmcondition.TypeSnapshotting, newVM.Status.Conditions)
			Expect(exists).To(BeFalse())
		})

		It("Should remove condition if it was present but no snapshots in progress", func() {
			vm := newVM()
			vm.Status.Conditions = []metav1.Condition{
				{
					Type:   vmcondition.TypeSnapshotting.String(),
					Status: metav1.ConditionTrue,
				},
			}
			fakeClient, resource, vmState = setupEnvironment(vm)
			reconcile()

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			_, exists := conditions.GetCondition(vmcondition.TypeSnapshotting, newVM.Status.Conditions)
			Expect(exists).To(BeFalse())
		})
	})
})
