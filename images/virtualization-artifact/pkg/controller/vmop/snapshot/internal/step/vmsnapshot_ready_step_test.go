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

package step

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
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
		It("should wait when snapshot annotation is not set", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			delete(vmop.Annotations, annotations.AnnVMOPSnapshotName)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewVMSnapshotReadyStep(fakeClient)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
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
})
