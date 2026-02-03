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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("WaitingDisksReadyStep", func() {
	var (
		ctx        context.Context
		fakeClient client.WithWatch
		recorder   *eventrecord.EventRecorderLoggerMock
		step       *WaitingDisksReadyStep
	)

	BeforeEach(func() {
		ctx = context.Background()
		recorder = newNoOpRecorder()
	})

	Describe("DryRun mode", func() {
		It("should skip for restore in DryRun mode", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Spec.Restore.Mode = v1alpha2.SnapshotOperationModeDryRun

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewWaitingDisksStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should skip for clone in DryRun mode", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Spec.Clone.Mode = v1alpha2.SnapshotOperationModeDryRun

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewWaitingDisksStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Describe("Waiting for disks", func() {
		It("should wait when disk is not ready for clone", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Kind: v1alpha2.VirtualDiskKind, Name: "test-disk"},
			}

			vd := createVirtualDisk("default", "test-disk", "test-vmop-uid", v1alpha2.DiskProvisioning)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vd)
			Expect(err).NotTo(HaveOccurred())

			step = NewWaitingDisksStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("should proceed when disk is ready for clone", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Kind: v1alpha2.VirtualDiskKind, Name: "test-disk"},
			}

			vd := createVirtualDisk("default", "test-disk", "test-vmop-uid", v1alpha2.DiskReady)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vd)
			Expect(err).NotTo(HaveOccurred())

			step = NewWaitingDisksStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should accept WaitForFirstConsumer phase for restore", func() {
			vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Kind: v1alpha2.VirtualDiskKind, Name: "test-disk"},
			}

			vd := createVirtualDisk("default", "test-disk", "test-vmop-uid", v1alpha2.DiskWaitForFirstConsumer)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vd)
			Expect(err).NotTo(HaveOccurred())

			step = NewWaitingDisksStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should return error when disk is in failed phase", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Kind: v1alpha2.VirtualDiskKind, Name: "test-disk"},
			}

			vd := createVirtualDisk("default", "test-disk", "test-vmop-uid", v1alpha2.DiskFailed)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vd)
			Expect(err).NotTo(HaveOccurred())

			step = NewWaitingDisksStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed phase"))
			Expect(result).NotTo(BeNil())
		})

		It("should be idempotent - multiple calls with same state return same result", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Kind: v1alpha2.VirtualDiskKind, Name: "test-disk"},
			}

			vd := createVirtualDisk("default", "test-disk", "test-vmop-uid", v1alpha2.DiskReady)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vd)
			Expect(err).NotTo(HaveOccurred())

			step = NewWaitingDisksStep(fakeClient, recorder)

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

		It("should be idempotent when waiting for disk", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			vmop.Status.Resources = []v1alpha2.SnapshotResourceStatus{
				{Kind: v1alpha2.VirtualDiskKind, Name: "test-disk"},
			}

			vd := createVirtualDisk("default", "test-disk", "test-vmop-uid", v1alpha2.DiskProvisioning)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, vd)
			Expect(err).NotTo(HaveOccurred())

			step = NewWaitingDisksStep(fakeClient, recorder)

			for i := range 5 {
				result, err := step.Take(ctx, vmop)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil(), "Iteration %d should return non-nil result (waiting)", i)
			}
		})
	})
})
