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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
)

var _ = Describe("ProcessCloneStep", func() {
	var (
		ctx        context.Context
		fakeClient client.WithWatch
		recorder   *eventrecord.EventRecorderLoggerMock
		step       *ProcessCloneStep
	)

	BeforeEach(func() {
		ctx = context.Background()
		recorder = newNoOpRecorder()
	})

	Describe("Snapshot annotation check", func() {
		It("should return error when snapshot annotation is not found", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			delete(vmop.Annotations, annotations.AnnVMOPSnapshotName)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessCloneStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot name annotation not found"))
			Expect(result).NotTo(BeNil())
		})

		It("should be idempotent - multiple calls with missing annotation return same error", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			delete(vmop.Annotations, annotations.AnnVMOPSnapshotName)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessCloneStep(fakeClient, recorder)

			_, err1 := step.Take(ctx, vmop)
			_, err2 := step.Take(ctx, vmop)
			_, err3 := step.Take(ctx, vmop)

			Expect(err1).To(HaveOccurred())
			Expect(err2).To(HaveOccurred())
			Expect(err3).To(HaveOccurred())
			Expect(err1.Error()).To(Equal(err2.Error()))
			Expect(err2.Error()).To(Equal(err3.Error()))
		})
	})

	Describe("Snapshot not found", func() {
		It("should wait when snapshot is not found", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessCloneStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})
	})

	Describe("Secret not found", func() {
		It("should wait when restorer secret is not found", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessCloneStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})
	})
})
