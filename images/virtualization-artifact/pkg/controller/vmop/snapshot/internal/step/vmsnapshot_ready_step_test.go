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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("VMSnapshotReadyStep", func() {
	var (
		ctx        context.Context
		fakeClient client.WithWatch
		step       *VMSnapshotReadyStep
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("Skip conditions", func() {
		It("should skip when snapshot condition is already CleanedUp", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
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

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Describe("Restore operation", func() {
		It("should return error when snapshot name is empty", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "")

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot name is empty"))
			Expect(result).NotTo(BeNil())
		})

		It("should return error when snapshot is not ready", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", false)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is not ready to use"))
			Expect(result).NotTo(BeNil())
		})

		It("should proceed when snapshot is ready", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should be idempotent - multiple calls with same state return same result", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)

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

		It("should be idempotent when snapshot is not ready", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", false)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)

			for i := range 5 {
				result, err := step.Take(ctx, vmop)
				Expect(err).To(HaveOccurred(), "Iteration %d should return error", i)
				Expect(err.Error()).To(ContainSubstring("is not ready to use"))
				Expect(result).NotTo(BeNil(), "Iteration %d should return non-nil result", i)
			}
		})
	})

	Describe("Clone operation", func() {
		It("should return error when snapshot annotation is not set", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			delete(vmop.Annotations, annotations.AnnVMOPSnapshotName)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot name annotation not found"))
			Expect(result).NotTo(BeNil())
		})

		It("should proceed when snapshot is ready", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	// A snapshot captured by the unified-snapshotter SDK controllers never writes
	// vmscondition.VirtualMachineSnapshotReadyType (only the core-owned "Ready" condition, on their own
	// timing) and has no snapshot Secret at all. status.captureState is what marks it as such —
	// restorer.IsUnifiedCapture, the same discriminator the read side uses.
	Describe("Unified-snapshotter captured snapshot", func() {
		createUnifiedVMSnapshot := func(namespace, name string, phase v1alpha2.VirtualMachineSnapshotPhase, boundContentName string) *v1alpha2.VirtualMachineSnapshot {
			return &v1alpha2.VirtualMachineSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: v1alpha2.VirtualMachineSnapshotSpec{VirtualMachineName: "test-vm"},
				Status: v1alpha2.VirtualMachineSnapshotStatus{
					CaptureState:             &v1alpha2.UnifiedSnapshotterCaptureState{},
					Phase:                    phase,
					BoundSnapshotContentName: boundContentName,
					// Deliberately no Conditions and no VirtualMachineSnapshotSecretName: the
					// unified-snapshotter path never populates either.
				},
			}
		}

		It("should return error when the snapshot has not reached Ready yet", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "unified-snapshot")
			snapshot := createUnifiedVMSnapshot("default", "unified-snapshot", v1alpha2.VirtualMachineSnapshotPhaseInProgress, "")

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is not ready to use"))
			Expect(result).NotTo(BeNil())
		})

		// Binding is asynchronous and transient: restorer.NewManifestReader treats the same condition as
		// common.ErrQueueing, so this must wait rather than fail the operation.
		It("should requeue, not fail, when Ready but not yet bound to a SnapshotContent", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "unified-snapshot")
			snapshot := createUnifiedVMSnapshot("default", "unified-snapshot", v1alpha2.VirtualMachineSnapshotPhaseReady, "")

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RequeueAfter).To(Equal(boundContentRequeueAfter))
		})

		It("should proceed when Ready and bound, despite no secret and no VirtualMachineSnapshotReadyType condition", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "unified-snapshot")
			snapshot := createUnifiedVMSnapshot("default", "unified-snapshot", v1alpha2.VirtualMachineSnapshotPhaseReady, "content-1")

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})
})
