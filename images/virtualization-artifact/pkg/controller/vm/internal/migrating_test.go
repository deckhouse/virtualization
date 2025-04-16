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
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("MigratingHandler", func() {
	const (
		name      = "vm-migrating"
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

	newKVVMI := func(migrationState *virtv1.VirtualMachineInstanceMigrationState) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: virtv1.VirtualMachineInstanceStatus{
				MigrationState: migrationState,
			},
		}
	}

	reconcile := func() {
		h := NewMigratingHandler()
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

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonVmIsMigrating.String()))
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

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			cond, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmcondition.ReasonLastMigrationFinishedWithError.String()))
			Expect(cond.Message).To(Equal("Network issues"))
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

			newVM := new(virtv2.VirtualMachine)
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			_, exists := conditions.GetCondition(vmcondition.TypeMigrating, newVM.Status.Conditions)
			Expect(exists).To(BeFalse())
		})
	})
})
