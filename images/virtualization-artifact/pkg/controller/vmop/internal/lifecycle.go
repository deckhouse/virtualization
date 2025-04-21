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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

// LifecycleHandler calculates status of the VirtualMachineOperation resource.
type LifecycleHandler struct {
	client     client.Client
	srvCreator SrvCreator
	recorder   eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(client client.Client, srvCreator SrvCreator, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		client:     client,
		srvCreator: srvCreator,
		recorder:   recorder,
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
	signalSendCond := conditions.NewConditionBuilder(vmopcondition.SignalSentType).
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

	vmopSRV, err := h.srvCreator(vmop)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Ignore if VMOP is in final state.
	if vmopSRV.IsFinalState() {
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

	if vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
		log.Debug("Operation in progress, check if VM is completed", "vm.phase", vm.Status.Phase, "vmop.phase", vmop.Status.Phase)
		return h.syncOperationComplete(ctx, vmop, vm, vmopSRV)
	}

	// At this point VMOP is in Pending phase, do some validation checks.

	// Fail if there is at least one other VirtualMachineOperation in progress.
	found, err := h.otherVMOPIsInProgress(ctx, vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if found {
		vmop.Status.Phase = virtv2.VMOPPhaseFailed
		h.recorder.Event(vmop, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, "Other VirtualMachineOperations are in progress")
		conditions.SetCondition(
			completedCond.
				Reason(vmopcondition.ReasonOtherOperationsAreInProgress).
				Status(metav1.ConditionFalse).
				Message("Other VirtualMachineOperations are in progress"),
			&vmop.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Fail if there is at least one other migration in progress.
	found, err = h.otherMigrationsAreInProgress(ctx, vmop)
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
	if !vmopSRV.IsApplicableForRunPolicy(vm.Spec.RunPolicy) {
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
	if !vmopSRV.IsApplicableForVMPhase(vm.Status.Phase) {
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

	return reconcile.Result{}, nil
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

// syncOperationComplete detects if operation is completed and VM has desired phase.
// TODO detect if VM is stuck to prevent infinite InProgress state.
func (h LifecycleHandler) syncOperationComplete(ctx context.Context, changed *virtv2.VirtualMachineOperation, vm *virtv2.VirtualMachine, vmopSRV service.Operation) (reconcile.Result, error) {
	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(changed.GetGeneration())

	// Check for complete.
	isComplete, failureMessage, err := vmopSRV.IsComplete(ctx)
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
	reason, err := vmopSRV.GetInProgressReason(ctx)
	if vm.Status.Phase == virtv2.MachinePending {
		conditions.SetCondition(
			completedCond.
				Reason(reason).
				Status(metav1.ConditionFalse).
				Message("The request to restart the VirtualMachine has been sent. "+
					"The VirtualMachine is currently in the 'Pending' phase. "+
					"We are waiting for it to enter the 'Starting' phase."),
			&changed.Status.Conditions)
	} else {
		conditions.SetCondition(
			completedCond.
				Reason(reason).
				Status(metav1.ConditionFalse).
				Message("Wait until operation completed"),
			&changed.Status.Conditions)
	}

	return reconcile.Result{}, err
}

func (h LifecycleHandler) isFinalState(vmop *virtv2.VirtualMachineOperation) bool {
	return vmop != nil && (vmop.Status.Phase == virtv2.VMOPPhaseCompleted ||
		vmop.Status.Phase == virtv2.VMOPPhaseFailed ||
		vmop.Status.Phase == virtv2.VMOPPhaseTerminating)
}

// otherVMOPIsInProgress check if there is at least one VMOP for the same VM in progress phase.
func (h LifecycleHandler) otherVMOPIsInProgress(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (bool, error) {
	var vmopList virtv2.VirtualMachineOperationList
	err := h.client.List(ctx, &vmopList, client.InNamespace(vmop.GetNamespace()))
	if err != nil {
		return false, err
	}

	for _, other := range vmopList.Items {
		// Ignore ourself.
		if other.GetName() == vmop.GetName() {
			continue
		}
		// Ignore VMOPs for different VMs.
		if other.Spec.VirtualMachine != vmop.Spec.VirtualMachine {
			continue
		}
		// Return true if other VMOP is in progress.
		if other.Status.Phase == virtv2.VMOPPhaseInProgress {
			return true, nil
		}
	}
	return false, nil
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
