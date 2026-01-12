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

package internal

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmbdaBuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

var _ = Describe("VirtualMachineReadyHandler Handle", func() {
	var (
		vmbda                 *v1alpha2.VirtualMachineBlockDeviceAttachment
		attachmentServiceMock AttachmentServiceMock
	)

	BeforeEach(func() {
		vmbda = vmbdaBuilder.NewEmpty("vmbda", "default")
		attachmentServiceMock = AttachmentServiceMock{
			GetVirtualMachineFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return nil, nil
			},
			GetKVVMIFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
				return nil, nil
			},
			GetKVVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
				return nil, nil
			},
		}
	})

	It("should set condition reason to unknown if deletion timestamp is not nil", func() {
		vmbda.DeletionTimestamp = ptr.To(metav1.Time{Time: time.Now()})
		result, err := NewVirtualMachineReadyHandler(&attachmentServiceMock).Handle(context.Background(), vmbda)
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).NotTo(HaveOccurred())

		vmReadyCondition, ok := conditions.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(vmReadyCondition.Reason).To(Equal(conditions.ReasonUnknown.String()))
	})

	It("should set condition to false if vm not found", func() {
		result, err := NewVirtualMachineReadyHandler(&attachmentServiceMock).Handle(context.Background(), vmbda)
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).NotTo(HaveOccurred())

		vmReadyCondition, ok := conditions.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(vmReadyCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(vmReadyCondition.Reason).To(Equal(vmbdacondition.VirtualMachineNotReady.String()))
	})

	DescribeTable("should set condition by vm phase if kvvm and kvvmi exists", func(phase v1alpha2.MachinePhase, expectedStatus metav1.ConditionStatus, expectedReason string) {
		attachmentServiceMock.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
			return &v1alpha2.VirtualMachine{
				Status: v1alpha2.VirtualMachineStatus{
					Phase: phase,
				},
			}, nil
		}
		attachmentServiceMock.GetKVVMFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
			return &virtv1.VirtualMachine{}, nil
		}
		attachmentServiceMock.GetKVVMIFunc = func(ctx context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
			return &virtv1.VirtualMachineInstance{}, nil
		}

		result, err := NewVirtualMachineReadyHandler(&attachmentServiceMock).Handle(context.Background(), vmbda)
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).NotTo(HaveOccurred())

		vmReadyCondition, ok := conditions.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(vmReadyCondition.Status).To(Equal(expectedStatus))
		Expect(vmReadyCondition.Reason).To(Equal(expectedReason))
	},
		Entry("Running", v1alpha2.MachineRunning, metav1.ConditionTrue, vmbdacondition.VirtualMachineReady.String()),
		Entry("Migrating", v1alpha2.MachineMigrating, metav1.ConditionTrue, vmbdacondition.VirtualMachineReady.String()),
		Entry("Stopping", v1alpha2.MachineStopping, metav1.ConditionFalse, vmbdacondition.NotAttached.String()),
		Entry("Stopped", v1alpha2.MachineStopped, metav1.ConditionFalse, vmbdacondition.NotAttached.String()),
		Entry("Starting", v1alpha2.MachineStarting, metav1.ConditionFalse, vmbdacondition.NotAttached.String()),
		Entry("Degraded", v1alpha2.MachineDegraded, metav1.ConditionFalse, vmbdacondition.VirtualMachineNotReady.String()),
		Entry("Terminating", v1alpha2.MachineTerminating, metav1.ConditionFalse, vmbdacondition.VirtualMachineNotReady.String()),
		Entry("Pause", v1alpha2.MachinePause, metav1.ConditionFalse, vmbdacondition.VirtualMachineNotReady.String()),
		Entry("Pending", v1alpha2.MachinePending, metav1.ConditionFalse, vmbdacondition.VirtualMachineNotReady.String()),
	)

	It("should return false if kvvm is not found", func() {
		attachmentServiceMock.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
			return &v1alpha2.VirtualMachine{
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
				},
			}, nil
		}
		attachmentServiceMock.GetKVVMFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
			return nil, nil
		}

		result, err := NewVirtualMachineReadyHandler(&attachmentServiceMock).Handle(context.Background(), vmbda)
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).NotTo(HaveOccurred())

		vmReadyCondition, ok := conditions.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(vmReadyCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(vmReadyCondition.Reason).To(Equal(vmbdacondition.VirtualMachineNotReady.String()))
	})

	It("should return false if kvvmi is not found", func() {
		attachmentServiceMock.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
			return &v1alpha2.VirtualMachine{
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
				},
			}, nil
		}
		attachmentServiceMock.GetKVVMFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
			return &virtv1.VirtualMachine{}, nil
		}
		attachmentServiceMock.GetKVVMIFunc = func(ctx context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
			return nil, nil
		}

		result, err := NewVirtualMachineReadyHandler(&attachmentServiceMock).Handle(context.Background(), vmbda)
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).NotTo(HaveOccurred())

		vmReadyCondition, ok := conditions.GetCondition(vmbdacondition.VirtualMachineReadyType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(vmReadyCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(vmReadyCondition.Reason).To(Equal(vmbdacondition.VirtualMachineNotReady.String()))
	})

	It("should return error if get virtual machine failed", func() {
		attachmentServiceMock.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
			return nil, errors.New("test error")
		}

		result, err := NewVirtualMachineReadyHandler(&attachmentServiceMock).Handle(context.Background(), vmbda)
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).To(HaveOccurred())
	})

	It("should return error if get kvvm failed", func() {
		attachmentServiceMock.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
			return &v1alpha2.VirtualMachine{
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
				},
			}, nil
		}
		attachmentServiceMock.GetKVVMFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
			return nil, errors.New("test error")
		}

		result, err := NewVirtualMachineReadyHandler(&attachmentServiceMock).Handle(context.Background(), vmbda)
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).To(HaveOccurred())
	})

	It("should return error if get kvvmi failed", func() {
		attachmentServiceMock.GetVirtualMachineFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
			return &v1alpha2.VirtualMachine{
				Status: v1alpha2.VirtualMachineStatus{
					Phase: v1alpha2.MachineRunning,
				},
			}, nil
		}
		attachmentServiceMock.GetKVVMFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
			return &virtv1.VirtualMachine{}, nil
		}
		attachmentServiceMock.GetKVVMIFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
			return nil, errors.New("test error")
		}

		result, err := NewVirtualMachineReadyHandler(&attachmentServiceMock).Handle(context.Background(), vmbda)
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(err).To(HaveOccurred())
	})
})
