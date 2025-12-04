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

package handler

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	vmsopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
)

const (
	name      = "vmsop"
	namespace = "vmsop"
)

var _ = Describe("LifecycleHandler", func() {
	var (
		ctx             context.Context
		fakeClient      client.WithWatch
		srv             *reconciler.Resource[*v1alpha2.VirtualMachineSnapshotOperation, v1alpha2.VirtualMachineSnapshotOperationStatus]
		recorderMock    *eventrecord.EventRecorderLoggerMock
		createOperation *CreateOperationExecutorMock

		vmsop  *v1alpha2.VirtualMachineSnapshotOperation
		vms    *v1alpha2.VirtualMachineSnapshot
		secret *corev1.Secret
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...any) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorderMock },
		}
		createOperation = &CreateOperationExecutorMock{
			ExecuteFunc: func(contextMoqParam context.Context, virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation, vmSnapshot *v1alpha2.VirtualMachineSnapshot, secret *corev1.Secret) error {
				return nil
			},
		}

		vmsop = vmsopbuilder.New(
			vmsopbuilder.WithName(name),
			vmsopbuilder.WithNamespace(namespace),
			vmsopbuilder.WithType(v1alpha2.VMSOPTypeCreateVirtualMachine),
			vmsopbuilder.WithVirtualMachineSnapshotName("snapshot"),
			vmsopbuilder.WithCreateVirtualMachine(&v1alpha2.VMSOPCreateVirtualMachineSpec{Mode: v1alpha2.SnapshotOperationModeDryRun}),
		)

		vms = vmsnapshotbuilder.New(
			vmsnapshotbuilder.WithName("snapshot"),
			vmsnapshotbuilder.WithNamespace(namespace),
			vmsnapshotbuilder.WithVirtualMachineName("vm"),
			vmsnapshotbuilder.WithVirtualMachineSnapshotSecretName("secret"),
			vmsnapshotbuilder.WithVirtualMachineSnapshotPhase(v1alpha2.VirtualMachineSnapshotPhaseReady),
		)

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"username": []byte("username"),
				"password": []byte("password"),
				"endpoint": []byte("endpoint"),
			},
		}
	})

	AfterEach(func() {
		fakeClient = nil
		srv = nil
	})

	It("should return handler name", func() {
		h := NewLifecycleHandler(fakeClient, createOperation, recorderMock)
		Expect(h.Name()).To(Equal(lifecycleHandlerName))
	})

	It("should set terminating phase when object is being deleted", func() {
		fakeClient, srv = setupEnvironment(vmsop)
		srv.Changed().DeletionTimestamp = ptr.To(metav1.Now())

		h := NewLifecycleHandler(fakeClient, createOperation, recorderMock)
		_, err := h.Handle(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())

		Expect(srv.Changed().Status.Phase).To(Equal(v1alpha2.VMSOPPhaseTerminating))
	})

	type vmsopWithSecondLifecycleArgs struct {
		expectedPhase           v1alpha2.VMSOPPhase
		shouldFail              bool
		shouldOtherSnapshotName string
		shouldOtherBeOlder      bool
		shouldOtherBeFailed     bool
		shouldOtherBeCompleted  bool
	}
	DescribeTable("Checking VMSOP lifecycle handler with 2 VMSOP",
		func(args vmsopWithSecondLifecycleArgs) {
			vms.UID = "snapshot"
			vms.CreationTimestamp = metav1.NewTime(time.Now())

			vmsop2 := vmsop.DeepCopy()
			vmsop2.UID = "vmsop-2"
			vmsop2.Name = "vmsop-2"
			vmsop2.Status.Phase = v1alpha2.VMSOPPhaseInProgress

			if args.shouldOtherSnapshotName != "" {
				vmsop2.Spec.VirtualMachineSnapshotName = args.shouldOtherSnapshotName
			}

			if args.shouldOtherBeOlder {
				vmsop2.CreationTimestamp = metav1.NewTime(vmsop2.CreationTimestamp.Add(-time.Hour))
			}

			if args.shouldOtherBeFailed {
				vmsop2.Status.Phase = v1alpha2.VMSOPPhaseFailed
			}

			if args.shouldOtherBeCompleted {
				vmsop2.Status.Phase = v1alpha2.VMSOPPhaseCompleted
			}

			fakeClient, srv = setupEnvironment(vmsop, vms, secret, vmsop2)

			h := NewLifecycleHandler(fakeClient, createOperation, recorderMock)
			_, err := h.Handle(ctx, srv.Changed())
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("VMSOP should fail if has operation in progress", vmsopWithSecondLifecycleArgs{
			shouldFail:         true,
			expectedPhase:      v1alpha2.VMSOPPhaseFailed,
			shouldOtherBeOlder: true,
		}),
		Entry("VMSOP shouldn't fail if has operation for different snapshot", vmsopWithSecondLifecycleArgs{
			shouldFail:              false,
			expectedPhase:           v1alpha2.VMSOPPhaseCompleted,
			shouldOtherSnapshotName: "snapshot0",
		}),
		Entry("VMSOP shouldn't fail if has other operation with failed phase", vmsopWithSecondLifecycleArgs{
			shouldFail:          false,
			expectedPhase:       v1alpha2.VMSOPPhaseCompleted,
			shouldOtherBeFailed: true,
		}),
		Entry("VMSOP shouldn't fail if has other operation with completedfailed phase", vmsopWithSecondLifecycleArgs{
			shouldFail:             false,
			expectedPhase:          v1alpha2.VMSOPPhaseCompleted,
			shouldOtherBeCompleted: true,
		}),
	)

	type vmsopLifecycleArgs struct {
		executeErr              error
		expectedPhase           v1alpha2.VMSOPPhase
		failedVMS               bool
		shouldCompleteAfterExec bool
		shouldFail              bool
		shouldNilVMS            bool
		shoildNilVMSSecret      bool
		shouldAlreadyFailed     bool
		shouldAlreadyCompleted  bool
	}
	DescribeTable("Checking VMSOP lifecycle handler",
		func(args vmsopLifecycleArgs) {
			if args.executeErr != nil {
				createOperation.ExecuteFunc = func(contextMoqParam context.Context, virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation, vmSnapshot *v1alpha2.VirtualMachineSnapshot, secret *corev1.Secret) error {
					return args.executeErr
				}
			}

			if args.shouldCompleteAfterExec {
				createOperation.ExecuteFunc = func(contextMoqParam context.Context, virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation, vmSnapshot *v1alpha2.VirtualMachineSnapshot, secret *corev1.Secret) error {
					return nil
				}
			}

			if args.failedVMS {
				vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
			}

			if args.shoildNilVMSSecret {
				secret = nil
			}

			if args.shouldAlreadyFailed {
				vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
				vmsop.Status.Conditions = []metav1.Condition{
					{
						Type:               string(vmsopcondition.TypeCompleted),
						Status:             metav1.ConditionFalse,
						Reason:             string(vmsopcondition.ReasonOperationFailed),
						Message:            "operation failed",
						LastTransitionTime: metav1.Now(),
					},
				}
			}

			if args.shouldAlreadyCompleted {
				vmsop.Status.Phase = v1alpha2.VMSOPPhaseCompleted
				vmsop.Status.Conditions = []metav1.Condition{
					{
						Type:               string(vmsopcondition.TypeCompleted),
						Status:             metav1.ConditionTrue,
						Reason:             string(vmsopcondition.ReasonOperationCompleted),
						Message:            "operation completed",
						LastTransitionTime: metav1.Now(),
					},
				}
			}

			fakeClient, srv = setupEnvironment(vmsop)
			if !args.shoildNilVMSSecret {
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())
			}
			if !args.shouldNilVMS {
				Expect(fakeClient.Create(ctx, vms)).To(Succeed())
			}

			h := NewLifecycleHandler(fakeClient, createOperation, recorderMock)
			_, err := h.Handle(ctx, srv.Changed())
			if args.shouldFail {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			if args.expectedPhase != "" {
				status := srv.Changed().Status
				Expect(status.Phase).To(Equal(args.expectedPhase))
			}
		},
		Entry("VMSOP should exit early for failed operation", vmsopLifecycleArgs{
			shouldFail:          false,
			expectedPhase:       "",
			shouldAlreadyFailed: true,
		}),
		Entry("VMSOP should exit early for completed operation", vmsopLifecycleArgs{
			shouldFail:             false,
			expectedPhase:          "",
			shouldAlreadyCompleted: true,
		}),
		Entry("VMSOP should execute without errors", vmsopLifecycleArgs{
			expectedPhase: v1alpha2.VMSOPPhaseCompleted,
		}),
		Entry("VMSOP should fail with no snapshot", vmsopLifecycleArgs{
			shouldFail:    false,
			shouldNilVMS:  true,
			expectedPhase: v1alpha2.VMSOPPhaseFailed,
		}),
		Entry("VMSOP should fail with failed snapshot", vmsopLifecycleArgs{
			shouldFail:         true,
			shoildNilVMSSecret: true,
			expectedPhase:      v1alpha2.VMSOPPhaseFailed,
		}),
		Entry("VMSOP should fail with nil snapshot secret", vmsopLifecycleArgs{
			shouldFail:    false,
			failedVMS:     true,
			expectedPhase: v1alpha2.VMSOPPhaseFailed,
		}),
		Entry("VMSOP should fail execute", vmsopLifecycleArgs{
			executeErr:    fmt.Errorf(""),
			shouldFail:    false,
			expectedPhase: v1alpha2.VMSOPPhaseFailed,
		}),
	)
})
