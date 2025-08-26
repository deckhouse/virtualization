/*
Copyright 2024 Flant JSC

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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/powerstate/internal/service"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmop/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

type Base interface {
	Init(vmop *v1alpha2.VirtualMachineOperation)
	ShouldExecuteOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (bool, error)
	FetchVirtualMachineOrSetFailedPhase(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*v1alpha2.VirtualMachine, error)
	IsApplicableOrSetFailedPhase(checker genericservice.ApplicableChecker, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) bool
}

type LifecycleHandler struct {
	client       client.Client
	svcOpCreator SvcOpCreator
	base         Base
	recorder     eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(client client.Client, svcOpCreator SvcOpCreator, base Base, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		client:       client,
		svcOpCreator: svcOpCreator,
		base:         base,
		recorder:     recorder,
	}
}

// Handle sets conditions depending on cluster state.
// It should set Running condition to start operation on VM.
func (h LifecycleHandler) Handle(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (reconcile.Result, error) {
	log, ctx := logger.GetHandlerContext(ctx, lifecycleHandlerName)

	// Do not update conditions for object in the deletion state.
	if commonvmop.IsTerminating(vmop) {
		vmop.Status.Phase = v1alpha2.VMOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	// Ignore if VMOP is in final state.
	if commonvmop.IsFinished(vmop) {
		return reconcile.Result{}, nil
	}

	// 1.Initialize new VMOP resource: set phase to Pending and all conditions to Unknown.
	h.base.Init(vmop)

	// 2. Get VirtualMachine for validation vmop.
	vm, err := h.base.FetchVirtualMachineOrSetFailedPhase(ctx, vmop)
	if vm == nil || err != nil {
		return reconcile.Result{}, err
	}

	svcOp, err := h.svcOpCreator(vmop)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 3. Operation already in progress. Check if the operation is completed.
	// Synchronize conditions to the VMOP.
	if isOperationInProgress(vmop) {
		log.Debug("Operation in progress, check if VM is completed", "vm.phase", vm.Status.Phase, "vmop.phase", vmop.Status.Phase)
		return reconcile.Result{}, h.syncOperationComplete(ctx, vmop, svcOp)
	}

	// 4. VMOP is not in progress.
	// All operations must be performed in course, check it and set phase if operation cannot be executed now.
	should, err := h.base.ShouldExecuteOrSetFailedPhase(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !should {
		return reconcile.Result{}, nil
	}

	// 5. Check if the operation is applicable for executed.
	isApplicable := h.base.IsApplicableOrSetFailedPhase(svcOp, vmop, vm)
	if !isApplicable {
		return reconcile.Result{}, nil
	}

	// 5. The Operation is valid, and can be executed.
	h.execute(ctx, vmop, vm, svcOp)

	return reconcile.Result{}, err
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

func (h LifecycleHandler) execute(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine, svcOp service.Operation) {
	log := logger.FromContext(ctx)

	h.recordEvent(ctx, vmop, vm)

	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(vmop.GetGeneration())
	signalSendCond := conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
		Generation(vmop.GetGeneration())

	// 1. Execute the operation.
	err := svcOp.Execute(ctx)
	if err != nil {
		// 1.1 If the operation fails, set vmop to failed.
		// The operation is never performed twice.
		failMsg := fmt.Sprintf("Sending signal %q to VM", vmop.Spec.Type)
		log.Debug(failMsg, logger.SlogErr(err))
		failMsg = fmt.Sprintf("%s: %v", failMsg, err)
		h.recorder.Event(vmop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed, failMsg)

		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonOperationFailed).
				Message(failMsg).
				Status(metav1.ConditionFalse),
			&vmop.Status.Conditions)
		conditions.SetCondition(
			signalSendCond.
				Reason(vmopcondition.ReasonSignalSentError).
				Status(metav1.ConditionFalse),
			&vmop.Status.Conditions)
	}

	// 2. The Operation is successfully executed.
	// Turn the phase to InProgress and set the send signal condition to true.
	msg := fmt.Sprintf("Sent signal %q to VM without errors.", vmop.Spec.Type)
	log.Debug(msg)
	h.recorder.Event(vmop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPInProgress, msg)

	vmop.Status.Phase = v1alpha2.VMOPPhaseInProgress

	reason := svcOp.GetInProgressReason()

	conditions.SetCondition(
		completedCond.
			Reason(reason).
			Message("Wait for operation to complete").
			Status(metav1.ConditionFalse),
		&vmop.Status.Conditions)
	conditions.SetCondition(
		signalSendCond.
			Reason(vmopcondition.ReasonSignalSentSuccess).
			Status(metav1.ConditionTrue),
		&vmop.Status.Conditions)
}

// syncOperationComplete detects if operation is completed and VM has desired phase.
func (h LifecycleHandler) syncOperationComplete(ctx context.Context, changed *v1alpha2.VirtualMachineOperation, svcOp service.Operation) error {
	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(changed.GetGeneration())

	isComplete, failureMessage, err := svcOp.IsComplete(ctx)
	if err != nil {
		return fmt.Errorf("check if operation is complete: %w", err)
	}

	if isComplete {
		if failureMessage != "" {
			changed.Status.Phase = v1alpha2.VMOPPhaseFailed
			h.recorder.Event(changed, corev1.EventTypeNormal, v1alpha2.ReasonErrVMOPFailed, "VirtualMachineOperation failed")
			conditions.SetCondition(
				completedCond.
					Reason(vmopcondition.ReasonOperationFailed).
					Status(metav1.ConditionFalse).
					Message(failureMessage),
				&changed.Status.Conditions)
			return nil
		}
		changed.Status.Phase = v1alpha2.VMOPPhaseCompleted
		h.recorder.Event(changed, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded, "VirtualMachineOperation succeeded")
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonOperationCompleted).
				Status(metav1.ConditionTrue).
				Message(""),
			&changed.Status.Conditions)
		return nil
	}

	// Keep InProgress phase as-is (InProgress), set complete condition to false.
	reason := svcOp.GetInProgressReason()
	conditions.SetCondition(
		completedCond.
			Reason(reason).
			Status(metav1.ConditionFalse).
			Message("Wait until operation completed"),
		&changed.Status.Conditions)

	return err
}

func (h LifecycleHandler) recordEvent(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation, vm *v1alpha2.VirtualMachine) {
	log := logger.FromContext(ctx)

	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeStart:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMStarted,
			"Start initiated with VirtualMachineOperation",
		)
	case v1alpha2.VMOPTypeStop:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMStopped,
			"Stop initiated with VirtualMachineOperation",
		)
	case v1alpha2.VMOPTypeRestart:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVMRestarted,
			"Restart initiated with VirtualMachineOperation",
		)
	}
}

func isOperationInProgress(vmop *v1alpha2.VirtualMachineOperation) bool {
	sent, _ := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	return sent.Status == metav1.ConditionTrue && completed.Status != metav1.ConditionTrue
}
