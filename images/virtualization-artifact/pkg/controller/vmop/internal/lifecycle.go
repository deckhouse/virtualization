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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/livemigration"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

// LifecycleHandler calculates status of the VirtualMachineOperation resource.
type LifecycleHandler struct {
	client       client.Client
	svcOpCreator SvcOpCreator
	recorder     eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(client client.Client, svcOpCreator SvcOpCreator, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		client:       client,
		svcOpCreator: svcOpCreator,
		recorder:     recorder,
	}
}

// Handle sets conditions depending on cluster state.
// It should set Running condition to start operation on VM.
func (h LifecycleHandler) Handle(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(lifecycleHandlerName))

	if vmop == nil {
		return reconcile.Result{}, nil
	}

	// Do not update conditions for object in the deletion state.
	if vmop.DeletionTimestamp != nil {
		vmop.Status.Phase = virtv2.VMOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(vmop.GetGeneration())
	signalSendCond := conditions.NewConditionBuilder(vmopcondition.TypeSignalSent).
		Generation(vmop.GetGeneration())

	// Initialize new VMOP resource: set label with vm name, set phase to Pending and all conditions to Unknown.
	if vmop.Status.Phase == "" {
		h.recorder.Event(vmop, corev1.EventTypeNormal, virtv2.ReasonVMOPStarted, "VirtualMachineOperation started")
		vmop.Status.Phase = virtv2.VMOPPhasePending
		// Add all conditions in unknown state.
		conditions.SetCondition(
			completedCond.
				Reason(conditions.ReasonUnknown).
				Message("").
				Status(metav1.ConditionUnknown),
			&vmop.Status.Conditions)
		conditions.SetCondition(
			signalSendCond.
				Reason(conditions.ReasonUnknown).
				Message("").
				Status(metav1.ConditionUnknown),
			&vmop.Status.Conditions)
	}

	svcOp, err := h.svcOpCreator(vmop)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Ignore if VMOP is in final state.
	if svcOp.IsFinalState() {
		return reconcile.Result{}, nil
	}

	// Pending if quota exceeded.
	isQuotaExceededDuringMigration, err := h.isKubeVirtMigrationRejectedDueToQuota(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if isQuotaExceededDuringMigration {
		h.recorder.Event(vmop, corev1.EventTypeWarning, virtv2.ReasonErrVMOPPending, "Project quota exceeded")
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonQuotaExceeded).
				Status(metav1.ConditionFalse).
				Message("Project quota exceeded"),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Get VM for Pending and InProgress checks.
	vm, err := object.FetchObject(ctx, types.NamespacedName{Name: vmop.Spec.VirtualMachine, Namespace: vmop.Namespace}, h.client, &virtv2.VirtualMachine{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("get VirtualMachine for VMOP: %w", err)
	}
	if vm == nil {
		h.recorder.Event(vmop, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, "VirtualMachine not found")
		vmop.Status.Phase = virtv2.VMOPPhaseFailed
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonVirtualMachineNotFound).
				Status(metav1.ConditionFalse).
				Message("VirtualMachine not found"),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}
	annotations.AddLabel(vmop, annotations.LabelVirtualMachineUID, string(vm.GetUID()))

	if isOperationInProgress(vmop) {
		log.Debug("Operation in progress, check if VM is completed", "vm.phase", vm.Status.Phase, "vmop.phase", vmop.Status.Phase)
		return h.syncOperationComplete(ctx, vmop, svcOp)
	}

	// At this point VMOP is in Pending phase, do some validation checks.

	can, err := h.canBeRun(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if can {
		vmop.Status.Phase = virtv2.VMOPPhasePending
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonReadyToBeExecuted).
				Message("VMOP is waiting to be executed.").
				Status(metav1.ConditionFalse),
			&vmop.Status.Conditions)
	} else {
		vmop.Status.Phase = virtv2.VMOPPhaseFailed
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonNotReadyToBeExecuted).
				Message("VMOP cannot be executed now. Previously created operation should finish first.").
				Status(metav1.ConditionFalse),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Fail if there is at least one other migration in progress.
	found, err := h.otherMigrationsAreInProgress(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if found {
		vmop.Status.Phase = virtv2.VMOPPhaseFailed
		h.recorder.Event(vmop, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, "Other Migrations are in progress")
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonOtherMigrationInProgress).
				Status(metav1.ConditionFalse).
				Message("Other Migrations are in progress"),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Fail if VirtualMachineOperation is not applicable for run policy.
	if !svcOp.IsApplicableForRunPolicy(vm.Spec.RunPolicy) {
		vmop.Status.Phase = virtv2.VMOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachine with runPolicy %s", vmop.Spec.Type, vm.Spec.RunPolicy)
		h.recorder.Event(vmop, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, failMsg)
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonNotApplicableForRunPolicy).
				Status(metav1.ConditionFalse).
				Message(failMsg),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Fail if VirtualMachineOperation is not applicable for VM phase.
	if !svcOp.IsApplicableForVMPhase(vm.Status.Phase) {
		vmop.Status.Phase = virtv2.VMOPPhaseFailed

		failMsg := fmt.Sprintf("Operation type %s is not applicable for VirtualMachine in phase %s", vmop.Spec.Type, vm.Status.Phase)
		h.recorder.Event(vmop, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, failMsg)
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonNotApplicableForVMPhase).
				Status(metav1.ConditionFalse).
				Message(failMsg),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Check if force flag is applicable for effective liveMigrationPolicy.
	msg, isApplicable := h.isApplicableForLiveMigrationPolicy(vmop, vm)

	if !isApplicable {
		vmop.Status.Phase = virtv2.VMOPPhaseFailed
		h.recorder.Event(vmop, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, msg)
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonNotApplicableForLiveMigrationPolicy).
				Status(metav1.ConditionFalse).
				Message(msg),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	} else if msg != "" {
		h.recorder.Event(vmop, corev1.EventTypeNormal, virtv2.ReasonVMOPStarted, msg)
	}

	return reconcile.Result{}, nil
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

// syncOperationComplete detects if operation is completed and VM has desired phase.
func (h LifecycleHandler) syncOperationComplete(ctx context.Context, changed *virtv2.VirtualMachineOperation, svcOp service.Operation) (reconcile.Result, error) {
	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(changed.GetGeneration())

	// Check for complete.
	isComplete, failureMessage, err := svcOp.IsComplete(ctx)
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
	reason, err := svcOp.GetInProgressReason(ctx)
	if commonvmop.IsMigration(changed) && reason != vmopcondition.ReasonMigrationPending {
		changed.Status.Phase = virtv2.VMOPPhaseInProgress
	}

	conditions.SetCondition(
		completedCond.
			Reason(reason).
			Status(metav1.ConditionFalse).
			Message("Wait until operation completed"),
		&changed.Status.Conditions)

	return reconcile.Result{}, err
}

// Should run oldest VMOP first.
// If reconciling VMOP is oldest, and it is not finished, it should be run.
func (h LifecycleHandler) canBeRun(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (bool, error) {
	var vmopList virtv2.VirtualMachineOperationList
	err := h.client.List(ctx, &vmopList, client.InNamespace(vmop.GetNamespace()))
	if err != nil {
		return false, err
	}

	for _, other := range vmopList.Items {
		if other.Spec.VirtualMachine != vmop.Spec.VirtualMachine {
			continue
		}
		if commonvmop.IsFinished(&other) {
			continue
		}
		if other.GetUID() == vmop.GetUID() {
			continue
		}

		if other.CreationTimestamp.Before(ptr.To(vmop.CreationTimestamp)) {
			return false, nil
		}
		if isOperationInProgress(&other) {
			return false, nil
		}
	}

	return true, nil
}

func isOperationInProgress(vmop *virtv2.VirtualMachineOperation) bool {
	sent, _ := conditions.GetCondition(vmopcondition.TypeSignalSent, vmop.Status.Conditions)
	completed, _ := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	return sent.Status == metav1.ConditionTrue && completed.Status != metav1.ConditionTrue
}

func (h LifecycleHandler) otherMigrationsAreInProgress(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (bool, error) {
	if !commonvmop.IsMigration(vmop) {
		return false, nil
	}
	migList := &virtv1.VirtualMachineInstanceMigrationList{}
	err := h.client.List(ctx, migList, client.InNamespace(vmop.GetNamespace()))
	if err != nil {
		return false, err
	}
	for _, mig := range migList.Items {
		if !mig.IsFinal() && mig.Spec.VMIName == vmop.Spec.VirtualMachine {
			return true, nil
		}
	}
	return false, nil
}

func (h LifecycleHandler) isKubeVirtMigrationRejectedDueToQuota(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (bool, error) {
	if !commonvmop.IsMigration(vmop) {
		return false, nil
	}

	kubevirtMigrationName := service.KubevirtMigrationName(vmop)
	kubevirtMigration, err := object.FetchObject(ctx, types.NamespacedName{
		Namespace: vmop.GetNamespace(),
		Name:      kubevirtMigrationName,
	}, h.client, &virtv1.VirtualMachineInstanceMigration{})
	if err != nil {
		return false, err
	}

	if kubevirtMigration == nil {
		return false, nil
	}

	kubevirtMigrationRejectedByResourceQuotaCondition := conditions.GetKVVMIMCondition(conditions.KubevirtMigrationRejectedByResourceQuotaType, kubevirtMigration.Status.Conditions)
	if kubevirtMigrationRejectedByResourceQuotaCondition != nil {
		return true, nil
	}

	return false, nil
}

func (h LifecycleHandler) isApplicableForLiveMigrationPolicy(vmop *virtv2.VirtualMachineOperation, vm *virtv2.VirtualMachine) (string, bool) {
	// No need to check live migration policy if operation is not related to migrations.
	if !commonvmop.IsMigration(vmop) {
		return "", true
	}

	// No problems if force flag is not specified.
	if vmop.Spec.Force == nil {
		return "", true
	}

	effectivePolicy, autoConverge, err := livemigration.CalculateEffectivePolicy(*vm, vmop)
	if err != nil {
		msg := fmt.Sprintf("Operation is invalid: %v", err)
		return msg, false
	}

	msg := fmt.Sprintf("Migration settings for operation type %s: liveMigrationPolicy %s, autoConverge %v", vmop.Spec.Type, effectivePolicy, autoConverge)
	return msg, true
}
