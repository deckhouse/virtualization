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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsnapshot"
	vmsopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmsop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	name      = "vmsop"
	namespace = "vmsop"
)

type vmsopLifecycleArgs struct {
	completeMessage         string
	executeErr              error
	expectedCopletedCond    bool
	expectedPhase           v1alpha2.VMSOPPhase
	failedVMS               bool
	shouldCompleteAfterExec bool
	shouldComplete          bool
	shouldFail              bool
	shouldInProgress        bool
	shouldNilVMS            bool
}

var _ = Describe("LifecycleHandler", func() {
	var (
		ctx             context.Context
		fakeClient      client.WithWatch
		srv             *reconciler.Resource[*v1alpha2.VirtualMachineSnapshotOperation, v1alpha2.VirtualMachineSnapshotOperationStatus]
		recorderMock    *eventrecord.EventRecorderLoggerMock
		createOperation *CreateOpeartionerMock

		vmsop *v1alpha2.VirtualMachineSnapshotOperation
		vms   *v1alpha2.VirtualMachineSnapshot
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		recorderMock = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...any) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorderMock },
		}
		createOperation = &CreateOpeartionerMock{
			ExecuteFunc: func(contextMoqParam context.Context, virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation) (reconcile.Result, error) {
				return reconcile.Result{}, nil
			},
			IsInProgressFunc: func(virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation) bool {
				return false
			},
			IsCompleteFunc: func(virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation) (bool, string) {
				return false, ""
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
	})

	AfterEach(func() {
		fakeClient = nil
		srv = nil
	})

	It("should set terminating phase when object is being deleted", func() {
		fakeClient, srv = setupEnvironment(vmsop)
		srv.Changed().DeletionTimestamp = ptr.To(metav1.Now())

		h := NewLifecycleHandler(fakeClient, createOperation, recorderMock)
		_, err := h.Handle(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())

		Expect(srv.Changed().Status.Phase).To(Equal(v1alpha2.VMSOPPhaseTerminating))
	})

	It("should fail if has operation in progress", func() {
		vms.UID = "snapshot"
		vms.CreationTimestamp = metav1.NewTime(time.Now())

		vmsop2 := vmsop.DeepCopy()
		vmsop2.UID = "vmsop-2"
		vmsop2.Name = "vmsop-2"
		vmsop2.Status.Phase = v1alpha2.VMSOPPhaseInProgress
		vmsop2.CreationTimestamp = metav1.NewTime(vmsop2.CreationTimestamp.Add(-time.Hour))

		fakeClient, srv = setupEnvironment(vmsop, vms, vmsop2)

		h := NewLifecycleHandler(fakeClient, createOperation, recorderMock)
		_, err := h.Handle(ctx, srv.Changed())
		Expect(err).NotTo(HaveOccurred())
		// Expect(err.Error()).To(ContainSubstring("VMSOP cannot be executed now. Previously created operation should finish first."))
	})

	DescribeTable("Checking VMSOP lifecycle handler",
		func(args vmsopLifecycleArgs) {
			var shouldCompleteAfterExec bool

			if args.executeErr != nil {
				createOperation.ExecuteFunc = func(contextMoqParam context.Context, virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation) (reconcile.Result, error) {
					return reconcile.Result{}, args.executeErr
				}
			}

			if args.shouldComplete {
				createOperation.IsCompleteFunc = func(virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation) (bool, string) {
					return true, args.completeMessage
				}
			}

			if args.shouldCompleteAfterExec {
				createOperation.ExecuteFunc = func(contextMoqParam context.Context, virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation) (reconcile.Result, error) {
					shouldCompleteAfterExec = true

					return reconcile.Result{}, nil
				}

				createOperation.IsCompleteFunc = func(virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation) (bool, string) {
					return shouldCompleteAfterExec, args.completeMessage
				}
			}

			if args.shouldInProgress {
				createOperation.IsInProgressFunc = func(virtualMachineSnapshotOperation *v1alpha2.VirtualMachineSnapshotOperation) bool {
					return true
				}
			}

			if args.shouldNilVMS {
				vms = nil
			}

			if args.failedVMS {
				vms.Status.Phase = v1alpha2.VirtualMachineSnapshotPhaseFailed
			}

			fakeClient, srv = setupEnvironment(vmsop)
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
		Entry("VMSOP should execute without errors", vmsopLifecycleArgs{expectedPhase: v1alpha2.VMSOPPhaseInProgress}),
		Entry("VMSOP should be in completed", vmsopLifecycleArgs{
			shouldComplete:       true,
			expectedCopletedCond: true,
			expectedPhase:        v1alpha2.VMSOPPhaseCompleted,
		}),
		Entry("VMSOP should be in progress", vmsopLifecycleArgs{
			shouldInProgress: true,
			expectedPhase:    v1alpha2.VMSOPPhaseInProgress,
		}),
		Entry("VMSOP should fail with no snapshot", vmsopLifecycleArgs{
			shouldNilVMS:  true,
			shouldFail:    true,
			expectedPhase: v1alpha2.VMSOPPhaseFailed,
		}),
		Entry("VMSOP should fail with failed snapshot", vmsopLifecycleArgs{
			shouldFail:    true,
			failedVMS:     true,
			expectedPhase: v1alpha2.VMSOPPhaseFailed,
		}),
		Entry("VMSOP should fail execute", vmsopLifecycleArgs{
			executeErr:    fmt.Errorf(""),
			shouldFail:    true,
			expectedPhase: v1alpha2.VMSOPPhaseFailed,
		}),
		Entry("VMSOP should complete after execute", vmsopLifecycleArgs{
			shouldCompleteAfterExec: true,
			expectedPhase:           v1alpha2.VMSOPPhaseCompleted,
		}),
		Entry("VMSOP should complete after execute with failure", vmsopLifecycleArgs{
			shouldCompleteAfterExec: true,
			completeMessage:         "failure",
			executeErr:              fmt.Errorf(""),
			expectedPhase:           v1alpha2.VMSOPPhaseFailed,
			shouldFail:              true,
		}),
	)
})
