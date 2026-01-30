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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("CreateSnapshotStep", func() {
	var (
		ctx        context.Context
		fakeClient client.WithWatch
		recorder   *eventrecord.EventRecorderLoggerMock
		step       *CreateSnapshotStep
	)

	BeforeEach(func() {
		ctx = context.Background()
		recorder = newNoOpRecorder()
	})

	Describe("Skip conditions", func() {
		It("should skip when snapshot condition is already True", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Conditions = []metav1.Condition{
				{
					Type:   string(vmopcondition.TypeSnapshotReady),
					Status: metav1.ConditionTrue,
					Reason: string(vmopcondition.ReasonSnapshotOperationReady),
				},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should skip when snapshot condition reason is CleanedUp", func() {
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

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Describe("Existing snapshot handling", func() {
		It("should set failed condition when snapshot is in Failed phase", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", false)
			snapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(result).NotTo(BeNil())

			snapshotCond, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(snapshotCond.Reason).To(Equal(string(vmopcondition.ReasonSnapshotFailed)))
		})

		It("should set ready condition when snapshot is in Ready phase", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)
			snapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseReady

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			snapshotCond, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(snapshotCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(snapshotCond.Reason).To(Equal(string(vmopcondition.ReasonSnapshotOperationReady)))
		})

		It("should set in progress condition when snapshot exists but not ready or failed", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", false)
			snapshot.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseInProgress

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))

			snapshotCond, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(snapshotCond.Reason).To(Equal(string(vmopcondition.ReasonSnapshotInProgress)))
		})

		It("should set in progress condition when snapshot annotation exists but snapshot not found", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))

			snapshotCond, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(snapshotCond.Reason).To(Equal(string(vmopcondition.ReasonSnapshotInProgress)))
		})
	})

	Describe("Snapshot discovery", func() {
		It("should add annotation when owned snapshot already exists", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "")
			delete(vmop.Annotations, annotations.AnnVMOPSnapshotName)

			snapshot := createVMSnapshot("default", "existing-snapshot", "test-secret", false)
			snapshot.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion:         v1alpha2.SchemeGroupVersion.String(),
					Kind:               v1alpha2.VirtualMachineOperationKind,
					Name:               vmop.Name,
					UID:                vmop.UID,
					BlockOwnerDeletion: ptr.To(true),
				},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(vmop.Annotations[annotations.AnnVMOPSnapshotName]).To(Equal("existing-snapshot"))
		})
	})

	Describe("Snapshot creation", func() {
		It("should create snapshot when none exists", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "")
			delete(vmop.Annotations, annotations.AnnVMOPSnapshotName)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))

			Expect(vmop.Annotations).NotTo(BeNil())
			Expect(vmop.Annotations[annotations.AnnVMOPSnapshotName]).NotTo(BeEmpty())

			snapshotCond, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(snapshotCond.Reason).To(Equal(string(vmopcondition.ReasonSnapshotInProgress)))

			var snapshotList v1alpha2.VirtualMachineSnapshotList
			err = fakeClient.List(ctx, &snapshotList, client.InNamespace("default"))
			Expect(err).NotTo(HaveOccurred())
			Expect(snapshotList.Items).To(HaveLen(1))

			createdSnapshot := snapshotList.Items[0]
			Expect(createdSnapshot.Spec.VirtualMachineName).To(Equal("test-vm"))
			Expect(createdSnapshot.Spec.RequiredConsistency).To(BeTrue())
			Expect(createdSnapshot.Annotations[annotations.AnnVMOPUID]).To(Equal(string(vmop.UID)))
		})

		It("should create snapshot with nil annotations", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "")
			vmop.Annotations = nil

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(vmop.Annotations).NotTo(BeNil())
			Expect(vmop.Annotations[annotations.AnnVMOPSnapshotName]).NotTo(BeEmpty())
		})
	})

	Describe("Idempotency", func() {
		It("should be idempotent - multiple calls with ready snapshot return same result", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Conditions = []metav1.Condition{
				{
					Type:   string(vmopcondition.TypeSnapshotReady),
					Status: metav1.ConditionTrue,
					Reason: string(vmopcondition.ReasonSnapshotOperationReady),
				},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewCreateSnapshotStep(fakeClient, recorder)

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
