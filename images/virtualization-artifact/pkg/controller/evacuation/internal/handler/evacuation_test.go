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
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("TestEvacuationHandler", func() {
	const (
		nodeName    = "worker-0"
		vmName      = "vm-evacuate"
		vmNamespace = "default"
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
	)

	AfterEach(func() {
		fakeClient = nil
	})

	newVM := func(needEvict bool) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(vmName, vmNamespace)
		vm.Status.Node = nodeName
		if needEvict {
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:   vmcondition.TypeEvictionRequired.String(),
				Status: metav1.ConditionTrue,
			})
		}
		return vm
	}

	newVMWithReason := func(reason vmcondition.EvictionRequiredReason) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(vmName, vmNamespace)
		vm.Status.Node = nodeName
		vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
			Type:   vmcondition.TypeEvictionRequired.String(),
			Status: metav1.ConditionTrue,
			Reason: reason.String(),
		})
		return vm
	}

	// A machine that only lacks a node to accept it keeps its migratability: the eviction is worth
	// creating, because a node may be reopened or freed at any moment.
	newVMWaitingForTarget := func(reason vmcondition.EvictionRequiredReason) *v1alpha2.VirtualMachine {
		vm := newVMWithReason(reason)
		vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
			Type:   vmcondition.TypeMigratable.String(),
			Status: metav1.ConditionTrue,
			Reason: vmcondition.ReasonWaitingForMigrationTarget.String(),
		})
		return vm
	}

	newApprovedNode := func() *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        nodeName,
				Annotations: map[string]string{annotations.AnnNodeVMRestartApproved: ""},
			},
			Spec: corev1.NodeSpec{Unschedulable: true},
		}
	}

	newEventRecorder := func() *eventrecord.EventRecorderLoggerMock {
		return &eventrecord.EventRecorderLoggerMock{
			EventfFunc: func(_ client.Object, _, _, _ string, _ ...interface{}) {},
			EventFunc:  func(_ client.Object, _, _, _ string) {},
		}
	}

	newVMOP := func(phase v1alpha2.VMOPPhase) *v1alpha2.VirtualMachineOperation {
		vmop := newEvacuationVMOP(vmName, vmNamespace)
		vmop.Status.Phase = phase
		return vmop
	}

	DescribeTable("Trigger Evacuate vm",
		func(vm *v1alpha2.VirtualMachine, vmop *v1alpha2.VirtualMachineOperation, shouldEvict bool) {
			fakeClient = setupEnvironment(vm, vmop)

			h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
				return nil
			}}, newEventRecorder())
			_, err := h.Handle(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			vmops := v1alpha2.VirtualMachineOperationList{}
			err = fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))
			Expect(err).NotTo(HaveOccurred())

			slices.SortFunc(vmops.Items, func(a, b v1alpha2.VirtualMachineOperation) int {
				return cmp.Compare(a.CreationTimestamp.UnixNano(), b.CreationTimestamp.UnixNano())
			})

			vmopCount := 0
			if vmop != nil {
				vmopCount++
			}

			if shouldEvict {
				Expect(len(vmops.Items)).To(Equal(vmopCount + 1))

				vmop := vmops.Items[len(vmops.Items)-1]
				Expect(vmop.Spec.Type).To(Equal(v1alpha2.VMOPTypeEvict))
				_, exists := vmop.GetAnnotations()[annotations.AnnVMOPEvacuation]
				Expect(exists).To(Equal(true))
			} else {
				Expect(len(vmops.Items)).To(Equal(vmopCount))
			}
		},
		Entry("Should create vmop because VM evicted", newVM(true), nil, true),
		Entry("Should do nothing", newVM(false), nil, false),
		Entry("Should do nothing because VM already migrating", newVM(true), newVMOP(v1alpha2.VMOPPhaseInProgress), false),
		Entry("Should create vmop because VM evicted but old vmop finished", newVM(true), newVMOP(v1alpha2.VMOPPhaseCompleted), true),
		// A node being prepared for maintenance is a warning to the owner, nothing leaves it yet.
		Entry("Should do nothing while the node is only being prepared", newVMWithReason(vmcondition.ReasonNodeUnderMaintenance), nil, false),
		// An evacuation of a machine that cannot be live migrated would only die on a timeout.
		Entry("Should do nothing when the machine cannot leave the node", newVMWithReason(vmcondition.ReasonEvictionBlocked), nil, false),
		// A machine that only misses a target is fit for migration, so the attempt keeps its chance
		// of a node being reopened or freed while the eviction lasts.
		Entry("Should create vmop while only a migration target is missing", newVMWaitingForTarget(vmcondition.ReasonEvictionRequired), nil, true),
	)

	DescribeTable("Restart the machine to release the node",
		func(vm *v1alpha2.VirtualMachine, node *corev1.Node, shouldRestart bool) {
			var extra []client.Object
			if node != nil {
				extra = append(extra, node)
			}
			fakeClient = setupEnvironment(vm, extra...)

			h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
				return nil
			}}, newEventRecorder())
			_, err := h.Handle(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			vmops := v1alpha2.VirtualMachineOperationList{}
			Expect(fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))).To(Succeed())

			if shouldRestart {
				Expect(vmops.Items).To(HaveLen(1))
				Expect(vmops.Items[0].Spec.Type).To(Equal(v1alpha2.VMOPTypeRestart))
				Expect(vmops.Items[0].GetName()).To(HavePrefix("node-maintenance-restart-"))
				_, marked := vmops.Items[0].GetAnnotations()[annotations.AnnVMOPNodeMaintenance]
				Expect(marked).To(BeTrue())
			} else {
				Expect(vmops.Items).To(BeEmpty())
			}
		},
		Entry("restarts a machine that cannot leave the node",
			newVMWithReason(vmcondition.ReasonRestartRequired), newApprovedNode(), true),
		// The node is read again, so a permission taken back a moment ago does not produce a restart
		// nobody allows any more, even while the condition still reports the old reason.
		Entry("restarts nothing once the permission is gone", newVMWithReason(vmcondition.ReasonRestartRequired), nil, false),
	)

	// The permission on the node never turns into a restart for a machine that keeps its
	// migratability: it leaves the node alive as soon as a target appears.
	It("evacuates instead of restarting while the machine can still be live migrated", func() {
		vm := newVMWaitingForTarget(vmcondition.ReasonEvictionRequired)
		fakeClient = setupEnvironment(vm, newApprovedNode())

		h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
			return nil
		}}, newEventRecorder())
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		vmops := v1alpha2.VirtualMachineOperationList{}
		Expect(fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))).To(Succeed())
		Expect(vmops.Items).To(HaveLen(1))
		Expect(vmops.Items[0].Spec.Type).To(Equal(v1alpha2.VMOPTypeEvict))
	})

	// A failed evacuation produces no further events, so the handler has to come back on its own:
	// otherwise the machine keeps a condition promising retries while nothing retries.
	DescribeTable("Retry a failed evacuation after the backoff",
		func(failedAgo time.Duration, shouldRetry bool) {
			vm := newVM(true)
			failed := newVMOP(v1alpha2.VMOPPhaseFailed)
			failed.SetName("evacuation-failed")
			conditions.SetCondition(
				conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
					Status(metav1.ConditionFalse).
					Reason(vmopcondition.ReasonOperationFailed).
					LastTransitionTime(time.Now().Add(-failedAgo)),
				&failed.Status.Conditions,
			)
			fakeClient = setupEnvironment(vm, failed)

			h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
				return nil
			}}, newEventRecorder())
			result, err := h.Handle(ctx, vm)
			Expect(err).NotTo(HaveOccurred())

			vmops := v1alpha2.VirtualMachineOperationList{}
			Expect(fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))).To(Succeed())

			if shouldRetry {
				Expect(vmops.Items).To(HaveLen(2))
			} else {
				Expect(vmops.Items).To(HaveLen(1))
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			}
		},
		Entry("waits while the backoff is not over", time.Duration(0), false),
		Entry("creates a new evacuation once the backoff is over", time.Hour, true),
	)

	It("gives up on a running evacuation when the restart is approved", func() {
		vm := newVMWithReason(vmcondition.ReasonRestartRequired)
		node := newApprovedNode()
		evacuation := newVMOP(v1alpha2.VMOPPhaseInProgress)
		// The fake client does not resolve generateName, and a deletion needs a name.
		evacuation.SetName("evacuation-running")
		fakeClient = setupEnvironment(vm, node, evacuation)

		h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
			return nil
		}}, newEventRecorder())
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		// The evacuation keeps its finalizer, so the deletion shows up as a deletion timestamp: the
		// next reconcile cancels the migration behind it and releases the object.
		stale := v1alpha2.VirtualMachineOperation{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(evacuation), &stale)).To(Succeed())
		Expect(stale.GetDeletionTimestamp().IsZero()).To(BeFalse())
	})

	It("does not start a second restart while the first one is running", func() {
		vm := newVMWithReason(vmcondition.ReasonRestartRequired)
		running := newNodeMaintenanceRestartVMOP(vmName, vmNamespace)
		running.Name = "node-maintenance-restart-running"
		running.Status.Phase = v1alpha2.VMOPPhaseInProgress
		fakeClient = setupEnvironment(vm, newApprovedNode(), running)

		h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
			return nil
		}}, newEventRecorder())
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		vmops := v1alpha2.VirtualMachineOperationList{}
		Expect(fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))).To(Succeed())
		Expect(vmops.Items).To(HaveLen(1))
	})

	// A restart that failed is not repeated: an identical second attempt fixes nothing, and the
	// node stays occupied either way — the aggregate alert asks a person to look.
	It("does not retry a restart that failed during the same maintenance", func() {
		vm := newVMWithReason(vmcondition.ReasonRestartRequired)
		vm.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now().Add(-time.Hour))

		failed := newNodeMaintenanceRestartVMOP(vmName, vmNamespace)
		failed.Name = "node-maintenance-restart-failed"
		failed.CreationTimestamp = metav1.NewTime(time.Now().Add(-30 * time.Minute))
		failed.Status.Phase = v1alpha2.VMOPPhaseFailed
		fakeClient = setupEnvironment(vm, newApprovedNode(), failed)

		h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
			return nil
		}}, newEventRecorder())
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		vmops := v1alpha2.VirtualMachineOperationList{}
		Expect(fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))).To(Succeed())
		Expect(vmops.Items).To(HaveLen(1))
	})

	// The next maintenance is a new decision of an administrator, so the attempt from the previous
	// one does not carry over.
	It("restarts again during the next maintenance", func() {
		vm := newVMWithReason(vmcondition.ReasonRestartRequired)
		vm.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now())

		failed := newNodeMaintenanceRestartVMOP(vmName, vmNamespace)
		failed.Name = "node-maintenance-restart-old"
		failed.CreationTimestamp = metav1.NewTime(time.Now().Add(-time.Hour))
		failed.Status.Phase = v1alpha2.VMOPPhaseFailed
		fakeClient = setupEnvironment(vm, newApprovedNode(), failed)

		h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
			return nil
		}}, newEventRecorder())
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		vmops := v1alpha2.VirtualMachineOperationList{}
		Expect(fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))).To(Succeed())
		Expect(vmops.Items).To(HaveLen(2))
	})

	// An operation started by the owner owns the machine until it finishes: the platform does not
	// stack its restart on top of it.
	It("waits while another operation is running on the machine", func() {
		vm := newVMWithReason(vmcondition.ReasonRestartRequired)

		ownerOperation := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "stop-by-owner", Namespace: vmNamespace},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type:           v1alpha2.VMOPTypeStop,
				VirtualMachine: vmName,
			},
			Status: v1alpha2.VirtualMachineOperationStatus{Phase: v1alpha2.VMOPPhaseInProgress},
		}
		fakeClient = setupEnvironment(vm, newApprovedNode(), ownerOperation)

		h := NewEvacuationHandler(fakeClient, &EvacuateCancelerMock{CancelFunc: func(_ context.Context, _, _ string) error {
			return nil
		}}, newEventRecorder())
		_, err := h.Handle(ctx, vm)
		Expect(err).NotTo(HaveOccurred())

		vmops := v1alpha2.VirtualMachineOperationList{}
		Expect(fakeClient.List(ctx, &vmops, client.InNamespace(vmNamespace))).To(Succeed())
		Expect(vmops.Items).To(HaveLen(1))
	})

	Context("Cancel Evacuation", func() {
		It("Should cancel evacuation", func() {
			expectErr := errors.New("expectErr")
			canceler := &EvacuateCancelerMock{
				CancelFunc: func(_ context.Context, _, _ string) error {
					return expectErr
				},
			}

			vmop := newVMOP(v1alpha2.VMOPPhaseInProgress)
			vmop.Name = "evacuation-12345"

			fakeClient = setupEnvironment(newVM(true), vmop)
			h := NewEvacuationHandler(fakeClient, canceler, newEventRecorder())

			err := fakeClient.Delete(ctx, vmop)
			Expect(err).NotTo(HaveOccurred())

			newVM := &v1alpha2.VirtualMachine{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: vmName, Namespace: vmNamespace}, newVM)
			Expect(err).NotTo(HaveOccurred())

			_, err = h.Handle(testutil.ContextBackgroundWithNoOpLogger(), newVM)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expectErr))
		})
	})
})
