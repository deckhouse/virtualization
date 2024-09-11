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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

var lifeCycleConditions = []vmopcondition.Type{
	vmopcondition.CompletedType,
	vmopcondition.SignalSentType,
}

// LifecycleHandler calculates status of the VirtualMachineOperation resource.
type LifecycleHandler struct {
	vmopSrv service.VMOperationService
}

func NewLifecycleHandler(vmopSrv service.VMOperationService) *LifecycleHandler {
	return &LifecycleHandler{
		vmopSrv: vmopSrv,
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

	defer func() {
		// Update observed generation if all conditions have equal generation.
		if changed == nil {
			return
		}
		if len(changed.Status.Conditions) == 0 {
			changed.Status.ObservedGeneration = changed.GetGeneration()
			return
		}
		gen := changed.Status.Conditions[0].ObservedGeneration
		for _, c := range changed.Status.Conditions {
			if gen != c.ObservedGeneration {
				return
			}
		}
		changed.Status.ObservedGeneration = gen
	}()

	// Do not update conditions for object in the deletion state.
	if changed.DeletionTimestamp != nil {
		changed.Status.Phase = virtv2.VMOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	// Initialize new VMOP resource: set phase to Pending and all conditions to Unknown.
	if changed.Status.Phase == "" {
		// TODO add label with vm name.
		changed.Status.Phase = virtv2.VMOPPhasePending
		// Add all conditions in unknown state.
		for _, condType := range lifeCycleConditions {
			service.SetCondition(metav1.Condition{
				Type:   condType.String(),
				Status: metav1.ConditionUnknown,
				Reason: vmopcondition.ReasonUnknown.String(),
			}, &changed.Status.Conditions)
		}
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
		changed.Status.Phase = virtv2.VMOPPhaseFailed
		service.SetCondition(metav1.Condition{
			Type:    vmopcondition.CompletedType.String(),
			Reason:  vmopcondition.ReasonVirtualMachineNotFound.String(),
			Status:  metav1.ConditionFalse,
			Message: "VirtualMachine not found",
		}, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	if changed.Status.Phase == virtv2.VMOPPhaseInProgress {
		log.Debug("Operation in progress, check if VM is completed", "vm.phase", vm.Status.Phase, "vmop.phase", changed.Status.Phase)
		return h.checkOperationComplete(changed, vm)
	}

	// At this point VMOP is in Pending phase, do some validation checks.

	// Fail if there is at least one other VirtualMachineOperation in progress.
	found, err := h.vmopSrv.OtherVMOPIsInProgress(ctx, changed)
	if err != nil {
		return reconcile.Result{}, err
	}
	if found {
		changed.Status.Phase = virtv2.VMOPPhaseFailed
		service.SetCondition(metav1.Condition{
			Type:    vmopcondition.CompletedType.String(),
			Reason:  vmopcondition.ReasonOtherOperationsAreInProgress.String(),
			Status:  metav1.ConditionFalse,
			Message: "Other VirtualMachineOperations are in progress",
		}, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Fail if VirtualMachineOperation is not applicable for run policy.
	if !h.vmopSrv.IsApplicableForRunPolicy(changed, vm.Spec.RunPolicy) {
		changed.Status.Phase = virtv2.VMOPPhaseFailed
		service.SetCondition(metav1.Condition{
			Type:    vmopcondition.CompletedType.String(),
			Reason:  vmopcondition.ReasonNotApplicableForRunPolicy.String(),
			Status:  metav1.ConditionFalse,
			Message: fmt.Sprintf("Operation type %s is not applicable for VirtualMachine with runPolicy %s", changed.Spec.Type, vm.Spec.RunPolicy),
		}, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Fail if VirtualMachineOperation is not applicable for VM phase.
	if !h.vmopSrv.IsApplicableForVMPhase(changed, vm.Status.Phase) {
		changed.Status.Phase = virtv2.VMOPPhaseFailed
		service.SetCondition(metav1.Condition{
			Type:    vmopcondition.CompletedType.String(),
			Reason:  vmopcondition.ReasonNotApplicableForVMPhase.String(),
			Status:  metav1.ConditionFalse,
			Message: fmt.Sprintf("Operation type %s is not applicable for VirtualMachine in phase %s", changed.Spec.Type, vm.Status.Phase),
		}, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

// checkOperationComplete detects if operation is completed and VM has desired phase.
// TODO detect if VM is stuck to prevent infinite InProgress state.
func (h LifecycleHandler) checkOperationComplete(changed *virtv2.VirtualMachineOperation, vm *virtv2.VirtualMachine) (reconcile.Result, error) {
	// Check for complete.
	if h.vmopSrv.IsComplete(changed, vm) {
		changed.Status.Phase = virtv2.VMOPPhaseCompleted
		service.SetCondition(metav1.Condition{
			Type:    vmopcondition.CompletedType.String(),
			Reason:  vmopcondition.ReasonOperationCompleted.String(),
			Message: "Virtual machine is in desired state",
			Status:  metav1.ConditionTrue,
		}, &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}

	// Keep InProgress phase as-is (InProgress), set complete condition to false.
	service.SetCondition(metav1.Condition{
		Type:    vmopcondition.CompletedType.String(),
		Reason:  h.vmopSrv.InProgressReasonForType(changed).String(),
		Message: "Wait until operation completed",
		Status:  metav1.ConditionFalse,
	}, &changed.Status.Conditions)
	return reconcile.Result{}, nil
}
