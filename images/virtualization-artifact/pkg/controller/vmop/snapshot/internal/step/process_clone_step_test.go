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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
			Expect(err2.Error()).To(Equal(err1.Error()), "err2 should be equal to the first error")
			Expect(err3.Error()).To(Equal(err1.Error()), "err3 should be equal to the first error")
		})
	})

	Describe("Snapshot not found", func() {
		It("should return error when snapshot is not found", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessCloneStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snapshot is not found"))
			Expect(result).NotTo(BeNil())
		})
	})

	Describe("Secret not found", func() {
		It("should return error when restorer secret is not found", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessCloneStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("restorer secret is not found"))
			Expect(result).NotTo(BeNil())
		})
	})

	Describe("Process completion", func() {
		It("should complete after running Process if all resources are in the Completed state", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)
			vm := createVirtualMachine("default", "test-vm", v1alpha2.MachineRunning)
			restorerSecret := createRestorerSecret("default", "test-secret", vm)

			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(vmop, snapshot, restorerSecret)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessCloneStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(vmop.Status.Resources).NotTo(BeEmpty())
			for _, status := range vmop.Status.Resources {
				Expect(status.Status).To(Equal(v1alpha2.SnapshotResourceStatusCompleted))
			}
		})

		It("should requeue after running Process if there are resources in the Completed state", func() {
			vmop := createCloneVMOP("default", "test-vmop", "test-vm", "test-snapshot")
			snapshot := createVMSnapshot("default", "test-snapshot", "test-secret", true)
			vm := createVirtualMachine("default", "test-vm", v1alpha2.MachineRunning)

			vmbda := createVMBDA("default", "test-vmbda", "test-vm")
			restorerSecret := createRestorerSecretWithVMBDAs("default", "test-secret", vm, []*v1alpha2.VirtualMachineBlockDeviceAttachment{vmbda})

			intercept := interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment); ok {
						return apierrors.NewConflict(
							schema.GroupResource{Resource: "virtualmachineblockdeviceattachments"},
							obj.GetName(),
							fmt.Errorf("the object has been modified"),
						)
					}
					return c.Create(ctx, obj, opts...)
				},
			}

			var err error
			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept, vmop, snapshot, restorerSecret)
			Expect(err).NotTo(HaveOccurred())

			step = NewProcessCloneStep(fakeClient, recorder)
			result, err := step.Take(ctx, vmop)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(reconcile.Result{}))
			Expect(vmop.Status.Resources).NotTo(BeEmpty())

			hasInProgress := false
			for _, status := range vmop.Status.Resources {
				if status.Status == v1alpha2.SnapshotResourceStatusInProgress {
					hasInProgress = true
				}
			}
			Expect(hasInProgress).To(BeTrue(), "expected at least one resource with InProgress status")
		})
	})
})
