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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("ProcessRestoreStep", func() {
	var (
		ctx        context.Context
		fakeClient client.WithWatch
		recorder   *eventrecord.EventRecorderLoggerMock
		step       *ProcessRestoreStep
	)

	BeforeEach(func() {
		ctx = context.Background()
		recorder = newNoOpRecorder()
	})

	Describe("Maintenance mode check", func() {
		It("Should pass if restore mode dryrun", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Spec.Restore.Mode = v1alpha2.SnapshotOperationModeDryRun

			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)
			vm := createVirtualMachine("default", "test-vm", v1alpha2.MachineRunning)
			restorerSecret := createRestorerSecret("default", "test-secret", vm)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot, restorerSecret)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessRestoreStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(vmop.Status.Resources).NotTo(BeNil())
		})

		It("should wait when maintenance condition is not found", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessRestoreStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))
		})

		It("should wait when maintenance condition is false", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			setMaintenanceCondition(vmop, metav1.ConditionFalse)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessRestoreStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))
		})

		It("should be idempotent - multiple calls with maintenance false return same result", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			setMaintenanceCondition(vmop, metav1.ConditionFalse)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessRestoreStep(fakeClient, recorder)

			result1, err1 := step.Take(ctx, vmop)
			result2, err2 := step.Take(ctx, vmop)
			result3, err3 := step.Take(ctx, vmop)

			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).NotTo(HaveOccurred())
			Expect(err3).NotTo(HaveOccurred())
			Expect(result1).NotTo(BeNil())
			Expect(result2).NotTo(BeNil())
			Expect(result3).NotTo(BeNil())
		})
	})

	Describe("Snapshot not found", func() {
		It("should return error when snapshot is not found", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			setMaintenanceCondition(vmop, metav1.ConditionTrue)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessRestoreStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot is not found"))
			Expect(result).NotTo(BeNil())
		})
	})

	Describe("Secret not found", func() {
		It("should return error when restorer secret is not found", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			setMaintenanceCondition(vmop, metav1.ConditionTrue)
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessRestoreStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("restorer secret is not found"))
			Expect(result).NotTo(BeNil())
		})
	})
})
