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

package step

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("ExitMaintenanceStep", func() {
	var (
		ctx        context.Context
		fakeClient client.WithWatch
		recorder   *eventrecord.EventRecorderLoggerMock
		step       *ExitMaintenanceStep
	)

	BeforeEach(func() {
		ctx = context.Background()
		recorder = newNoOpRecorder()
	})

	Describe("Skip conditions", func() {
		It("should skip when mode is DryRun", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Spec.Restore.Mode = v1alpha2.SnapshotOperationModeDryRun

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewExitMaintenanceStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should wait when VMOP maintenance condition is not found", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vm := createVirtualMachine("default", "test-vm", v1alpha2.MachineStopped)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vm)
			Expect(err).NotTo(HaveOccurred())

			step = NewExitMaintenanceStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))
		})

		It("should wait when VMOP maintenance condition is false", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			setMaintenanceCondition(vmop, metav1.ConditionFalse)
			vm := createVirtualMachine("default", "test-vm", v1alpha2.MachineStopped)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vm)
			Expect(err).NotTo(HaveOccurred())

			step = NewExitMaintenanceStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))
		})
	})

	Describe("VM maintenance mode exit", func() {
		It("should set VM maintenance to false when VM has maintenance condition true", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			setMaintenanceCondition(vmop, metav1.ConditionTrue)
			vm := createVirtualMachine("default", "test-vm", v1alpha2.MachineStopped)
			setVMMaintenanceCondition(vm, metav1.ConditionTrue, vmcondition.ReasonMaintenanceRestore)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vm)
			Expect(err).NotTo(HaveOccurred())

			step = NewExitMaintenanceStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			var updatedVM v1alpha2.VirtualMachine
			err = fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-vm"}, &updatedVM)
			Expect(err).NotTo(HaveOccurred())

			maintenanceCond, found := conditions.GetCondition(vmcondition.TypeMaintenance, updatedVM.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(maintenanceCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should set VMOP maintenance to false when VM has no maintenance condition", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			setMaintenanceCondition(vmop, metav1.ConditionTrue)
			vm := createVirtualMachine("default", "test-vm", v1alpha2.MachineStopped)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vm)
			Expect(err).NotTo(HaveOccurred())

			step = NewExitMaintenanceStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			maintenanceCond, found := conditions.GetCondition(vmopcondition.TypeMaintenanceMode, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(maintenanceCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(maintenanceCond.Reason).To(Equal(string(vmopcondition.ReasonMaintenanceModeDisabled)))
		})

		It("should set VMOP maintenance to false when VM maintenance is already false", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			setMaintenanceCondition(vmop, metav1.ConditionTrue)
			vm := createVirtualMachine("default", "test-vm", v1alpha2.MachineStopped)
			setVMMaintenanceCondition(vm, metav1.ConditionFalse, vmcondition.ReasonMaintenanceRestore)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vm)
			Expect(err).NotTo(HaveOccurred())

			step = NewExitMaintenanceStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			maintenanceCond, found := conditions.GetCondition(vmopcondition.TypeMaintenanceMode, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(maintenanceCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Describe("Idempotency", func() {
		It("should be idempotent - multiple calls with DryRun mode return same result", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Spec.Restore.Mode = v1alpha2.SnapshotOperationModeDryRun

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewExitMaintenanceStep(fakeClient, recorder)

			result1, err1 := step.Take(ctx, vmop)
			result2, err2 := step.Take(ctx, vmop)
			result3, err3 := step.Take(ctx, vmop)

			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).NotTo(HaveOccurred())
			Expect(err3).NotTo(HaveOccurred())
			Expect(result1).To(BeNil())
			Expect(result2).To(BeNil())
			Expect(result3).To(BeNil())
		})
	})
})
