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

package internal

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

// LifecycleHandler calculates status of the VirtualMachineOperation resource.
type LifecycleHandler struct {
	vmopSrv  service.VMOperationService
	recorder eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(recorder eventrecord.EventRecorderLogger, vmopSrv service.VMOperationService) *LifecycleHandler {
	return &LifecycleHandler{
		vmopSrv:  vmopSrv,
		recorder: recorder,
	}
}

// Handle sets conditions depending on cluster state.
// It should set Running condition to start operation on VM.
func (h LifecycleHandler) Handle(ctx context.Context, s state.VMOperationState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(lifecycleHandlerName))

	vmop := s.VirtualMachineOperation()
	if vmop == nil {
		return reconcile.Result{}, nil
	}

	changed := vmop.Changed()

	// Do not update conditions for object in the deletion state.
	if changed.DeletionTimestamp != nil {
		changed.Status.Phase = virtv2.VMOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(changed.GetGeneration())
	signalSendCond := conditions.NewConditionBuilder(vmopcondition.SignalSentType).
		Generation(changed.GetGeneration())

	// Initialize new VMOP resource: set label with vm name, set phase to Pending and all conditions to Unknown.
	if changed.Status.Phase == "" {
		h.recorder.Event(changed, corev1.EventTypeNormal, virtv2.ReasonVMOPStarted, "VirtualMachineOperation started")
		changed.Status.Phase = virtv2.VMOPPhasePending
		// Add all conditions in unknown state.
		conditions.SetCondition(
			completedCond.
				Reason(conditions.ReasonUnknown).
				Message("").
				Status(metav1.ConditionUnknown),
			&changed.Status.Conditions)
		conditions.SetCondition(
			signalSendCond.
				Reason(conditions.ReasonUnknown).
				Message("").
				Status(metav1.ConditionUnknown),
			&changed.Status.Conditions)
	}

	// Ignore if VMOP is in final state.
	if h.vmopSrv.IsFinalState(changed) {
		return reconcile.Result{}, nil
	}

	// Get VM for Pending and InProgress checks.
	vm, err := s.VirtualMachine(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("get VirtualMachine for VMOP: %w", err)
	}
	if vm == nil {
		h.recorder.Event(changed, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, "VirtualMachine not found")
		changed.Status.Phase = virtv2.VMOPPhaseFailed
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonVirtualMachineNotFound).
				Status(metav1.ConditionFalse).
				Message("VirtualMachine not found"),
			&changed.Status.Conditions)
		return reconcile.Result{}, nil
	}
	annotations.AddLabel(changed, annotations.LabelVirtualMachineUID, string(vm.GetUID()))

	if changed.Status.Phase == virtv2.VMOPPhaseInProgress {
		log.Debug("Operation in progress, check if VM is completed", "vm.phase", vm.Status.Phase, "vmop.phase", changed.Status.Phase)
		return h.syncOperationComplete(ctx, changed, vm)
	}

	// At this point VMOP is in Pending phase, do some validation checks.

	// Fail if there is at least one other VirtualMachineOperation in progress.
	found, err := h.vmopSrv.OtherVMOPIsInProgress(ctx, changed)
	if err != nil {
		return reconcile.Result{}, err
	}
	if found {
		changed.Status.Phase = virtv2.VMOPPhaseFailed
		h.recorder.Event(changed, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, "Other VirtualMachineOperations are in progress")
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonOtherOperationsAreInProgress).
				Status(metav1.ConditionFalse).
				Message("Other VirtualMachineOperations are in progress"),
			&changed.Status.Conditions)
		return reconcile.Result{}, nil
	}
	if isMigration(changed) {
		// Fail if there is at least one other migration in progress.
		found, err = h.vmopSrv.OtherMigrationsAreInProgress(ctx, changed)
		if err != nil {
			return reconcile.Result{}, err
		}
		if found {
			changed.Status.Phase = virtv2.VMOPPhaseFailed
			h.recorder.Event(changed, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, "Other Migrations are in progress")
			conditions.SetCondition(
				completedCond.
					Reason(vmopcondition.ReasonOtherMigrationInProgress).
					Status(metav1.ConditionFalse).
					Message("Other Migrations are in progress"),
				&changed.Status.Conditions)
			return reconcile.Result{}, nil
		}
	}

	// Fail if VirtualMachineOperation is not applicable for run policy.
	if !h.vmopSrv.IsApplicableForRunPolicy(changed, vm.Spec.RunPolicy) {
		changed.Status.Phase = virtv2.VMOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachine with runPolicy %s", changed.Spec.Type, vm.Spec.RunPolicy)
		h.recorder.Event(changed, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, failMsg)
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonNotApplicableForRunPolicy).
				Status(metav1.ConditionFalse).
				Message(failMsg),
			&changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Fail if VirtualMachineOperation is not applicable for VM phase.
	if !h.vmopSrv.IsApplicableForVMPhase(changed, vm.Status.Phase) {
		changed.Status.Phase = virtv2.VMOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachine in phase %s", changed.Spec.Type, vm.Status.Phase)
		h.recorder.Event(changed, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, failMsg)
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonNotApplicableForVMPhase).
				Status(metav1.ConditionFalse).
				Message(failMsg),
			&changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

// syncOperationComplete detects if operation is completed and VM has desired phase.
// TODO detect if VM is stuck to prevent infinite InProgress state.
func (h LifecycleHandler) syncOperationComplete(ctx context.Context, changed *virtv2.VirtualMachineOperation, vm *virtv2.VirtualMachine) (reconcile.Result, error) {
	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(changed.GetGeneration())

	// Check for complete.
	isComplete, failureMessage, err := h.vmopSrv.IsComplete(ctx, changed, vm)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("check if operation is complete: %w", err)
	}

	if isComplete {
		if failureMessage != "" {
			changed.Status.Phase = virtv2.VMOPPhaseFailed
			h.recorder.Event(changed, corev1.EventTypeNormal, virtv2.ReasonErrVMOPFailed, "VirtualMachineOperation failed")
			conditions.SetCondition(
				completedCond.
					Reason(vmopcondition.ReasonOperationFailed).
					Status(metav1.ConditionFalse).
					Message(failureMessage),
				&changed.Status.Conditions)
			return reconcile.Result{}, nil
		}
		changed.Status.Phase = virtv2.VMOPPhaseCompleted
		h.recorder.Event(changed, corev1.EventTypeNormal, virtv2.ReasonVMOPSucceeded, "VirtualMachineOperation succeeded")
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonOperationCompleted).
				Status(metav1.ConditionTrue).
				Message(""),
			&changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Keep InProgress phase as-is (InProgress), set complete condition to false.
	if vm.Status.Phase == virtv2.MachinePending {
		conditions.SetCondition(
			completedCond.
				Reason(h.vmopSrv.InProgressReasonForType(changed)).
				Status(metav1.ConditionFalse).
				Message("The request to restart the VirtualMachine has been sent. "+
					"The VirtualMachine is currently in the 'Pending' phase. "+
					"We are waiting for it to enter the 'Starting' phase."),
			&changed.Status.Conditions)
	} else {
		conditions.SetCondition(
			completedCond.
				Reason(h.vmopSrv.InProgressReasonForType(changed)).
				Status(metav1.ConditionFalse).
				Message("Wait until operation completed"),
			&changed.Status.Conditions)
	}

	return reconcile.Result{}, nil
}
