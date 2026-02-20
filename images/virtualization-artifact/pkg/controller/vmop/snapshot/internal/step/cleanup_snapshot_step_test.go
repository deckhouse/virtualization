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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("CleanupSnapshotStep", func() {
	var (
		ctx        context.Context
		fakeClient client.WithWatch
		recorder   *eventrecord.EventRecorderLoggerMock
		step       *CleanupSnapshotStep
	)

	BeforeEach(func() {
		ctx = context.Background()
		recorder = newNoOpRecorder()
	})

	Describe("Skip conditions", func() {
		It("should skip when clone mode is DryRun", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Spec.Clone.Mode = v1alpha2.SnapshotOperationModeDryRun

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCleanupSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should skip when snapshot condition is already CleanedUp", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Conditions = []metav1.Condition{
				{
					Type:   string(vmopcondition.TypeSnapshotReady),
					Status: metav1.ConditionFalse,
					Reason: string(vmopcondition.ReasonSnapshotCleanedUp),
				},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCleanupSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should skip when not all resources are completed", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Name: "disk-1", Status: v1alpha2.SnapshotResourceStatusCompleted},
				{Name: "disk-2", Status: v1alpha2.SnapshotResourceStatusInProgress},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCleanupSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))
		})

		It("should skip when snapshot name annotation is not present", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			delete(vmop.Annotations, annotations.AnnVMOPSnapshotName)
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Name: "disk-1", Status: v1alpha2.SnapshotResourceStatusCompleted},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCleanupSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Describe("Snapshot cleanup", func() {
		It("should set CleanedUp condition when snapshot is not found", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Name: "disk-1", Status: v1alpha2.SnapshotResourceStatusCompleted},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCleanupSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))

			snapshotCond, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(snapshotCond.Reason).To(Equal(string(vmopcondition.ReasonSnapshotCleanedUp)))
		})

		It("should delete snapshot when it exists and is not terminating", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Name: "disk-1", Status: v1alpha2.SnapshotResourceStatusCompleted},
			}
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewCleanupSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			snapshotCond, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(snapshotCond.Reason).To(Equal(string(vmopcondition.ReasonSnapshotCleanedUp)))

			var deletedSnapshot v1alpha2.VirtualMachineSnapshot
			err = fakeClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-snapshot"}, &deletedSnapshot)
			Expect(err).To(HaveOccurred())
		})

		It("should set CleanedUp condition when snapshot is terminating", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Name: "disk-1", Status: v1alpha2.SnapshotResourceStatusCompleted},
			}
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)
			now := metav1.Now()
			snapshot.DeletionTimestamp = &now
			snapshot.Finalizers = []string{"test-finalizer"}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewCleanupSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			snapshotCond, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(snapshotCond.Reason).To(Equal(string(vmopcondition.ReasonSnapshotCleanedUp)))
		})
	})

	Describe("Idempotency", func() {
		It("should be idempotent - multiple calls with CleanedUp condition return same result", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Conditions = []metav1.Condition{
				{
					Type:   string(vmopcondition.TypeSnapshotReady),
					Status: metav1.ConditionFalse,
					Reason: string(vmopcondition.ReasonSnapshotCleanedUp),
				},
			}
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Name: "disk-1", Status: v1alpha2.SnapshotResourceStatusCompleted},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCleanupSnapshotStep(fakeClient, recorder)

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
