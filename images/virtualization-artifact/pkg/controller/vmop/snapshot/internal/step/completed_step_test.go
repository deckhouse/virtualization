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

	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("CompletedStep", func() {
	var (
		ctx      context.Context
		recorder *eventrecord.EventRecorderLoggerMock
		step     *CompletedStep
	)

	BeforeEach(func() {
		ctx = context.Background()
		recorder = newNoOpRecorder()
	})

	It("should set completed phase and condition", func() {
		vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")

		step = NewCompletedStep(recorder)
		result, err := step.Take(ctx, vmop)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseCompleted))
	})

	It("should be idempotent - multiple calls produce same result", func() {
		vmop := createRestoreVMOP("default", "test-vmop", "test-vm", "test-snapshot")

		step = NewCompletedStep(recorder)

		result1, err1 := step.Take(ctx, vmop)
		phase1 := vmop.Status.Phase
		conditions1 := len(vmop.Status.Conditions)

		result2, err2 := step.Take(ctx, vmop)
		phase2 := vmop.Status.Phase
		conditions2 := len(vmop.Status.Conditions)

		result3, err3 := step.Take(ctx, vmop)
		phase3 := vmop.Status.Phase
		conditions3 := len(vmop.Status.Conditions)

		Expect(err1).NotTo(HaveOccurred())
		Expect(err2).NotTo(HaveOccurred())
		Expect(err3).NotTo(HaveOccurred())
		Expect(result1).NotTo(BeNil())
		Expect(result2).NotTo(BeNil())
		Expect(result3).NotTo(BeNil())

		Expect(phase1).To(Equal(phase2))
		Expect(phase2).To(Equal(phase3))
		Expect(phase1).To(Equal(v1alpha2.VMOPPhaseCompleted))

		Expect(conditions1).To(Equal(conditions2))
		Expect(conditions2).To(Equal(conditions3))
	})
})
