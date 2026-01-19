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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	vmservice "github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("MigratingHandler", func() {
	const (
		name      = "vm-migrating"
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

	newVM := func() *v1alpha2.VirtualMachine {
		return vmbuilder.NewEmpty(name, namespace)
	}

	newKVVMI := func(migrationState *virtv1.VirtualMachineInstanceMigrationState) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.MigrationState = migrationState
		return kvvmi
	}

	newVMOP := func(phase v1alpha2.VMOPPhase, reason string, isSignalSent bool) *v1alpha2.VirtualMachineOperation {
		vmop := vmopbuilder.New(
			vmopbuilder.WithGenerateName("test-vmop-"),
			vmopbuilder.WithNamespace(namespace),
			vmopbuilder.WithVirtualMachine(name),
			vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
		)
		vmop.Status.Phase = phase
		vmop.Status.Conditions = []metav1.Condition{
			{
				Type:   vmopcondition.TypeCompleted.String(),
				Status: metav1.ConditionFalse,
				Reason: reason,
			},
		}
		if isSignalSent {
			vmop.Status.Conditions = append(vmop.Status.Conditions, metav1.Condition{
				Type:   vmopcondition.TypeSignalSent.String(),
				Status: metav1.ConditionTrue,
			})
		}
		return vmop
	}

	reconcile := func() {
		h := NewMigratingHandler(vmservice.NewMigrationVolumesService(fakeClient, MakeKVVMFromVMSpec, 10*time.Second))
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		err = resource.Update(context.Background())
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Condition presence and absence scenarios", func() {
		It("Should display migrating condition when migration is in progress", func() {
			vm := newVM()
			migrationState := &virtv1.VirtualMachineInstanceMigrationState{
				StartTimestamp: &metav1.Time{Time: time.Now()},
				EndTimestamp:   nil,
			}
			kvvmi := newKVVMI(migrationState)
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonMigratingInProgress.String()))
		})

		It("Should display condition for last unsuccessful migration", func() {
			vm := newVM()
			migrationState := &virtv1.VirtualMachineInstanceMigrationState{
				StartTimestamp: &metav1.Time{Time: time.Now()},
				EndTimestamp:   &metav1.Time{Time: time.Now()},
				Failed:         true,
				FailureReason:  "Network issues",
			}
			kvvmi := newKVVMI(migrationState)
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			_, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeFalse())
		})

		It("Should remove migrating condition when migration is not in progress", func() {
			vm := newVM()
			vm.Status.Conditions = []metav1.Condition{
				{
					Type:   vmcondition.TypeMigrating.String(),
					Status: metav1.ConditionTrue,
				},
			}
			kvvmi := newKVVMI(nil)
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi)
			reconcile()

			newVM := &v1alpha2.VirtualMachine{}

			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			_, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeFalse())
		})

		It("Should set condition when vmop is in progress with pending reason", func() {
			vm := newVM()
			kvvmi := newKVVMI(nil)
			vmop := newVMOP(v1alpha2.VMOPPhaseInProgress, vmopcondition.ReasonMigrationPending.String(), true)
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi, vmop)

			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonMigratingPending.String()))
			Expect(cond.Message).To(Equal("Migration is awaiting start."))
		})

		It("Should set condition when vmop is in progress with target ready reason", func() {
			vm := newVM()
			kvvmi := newKVVMI(nil)
			vmop := newVMOP(v1alpha2.VMOPPhaseInProgress, vmopcondition.ReasonMigrationTargetReady.String(), true)
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi, vmop)

			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonMigratingPending.String()))
			Expect(cond.Message).To(Equal("Migration is awaiting execution."))
		})

		It("Should set condition when vmop is in progress with running reason", func() {
			vm := newVM()
			kvvmi := newKVVMI(nil)
			vmop := newVMOP(v1alpha2.VMOPPhaseInProgress, vmopcondition.ReasonMigrationRunning.String(), true)
			fakeClient, resource, vmState = setupEnvironment(vm, kvvmi, vmop)

			reconcile()

			newVM := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonMigratingInProgress.String()))
		})
	})
})
