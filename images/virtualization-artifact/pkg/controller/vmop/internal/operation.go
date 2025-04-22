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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const operationHandlerName = "OperationHandler"

// OperationHandler performs operation on Virtual Machine.
type OperationHandler struct {
	client       client.Client
	recorder     eventrecord.EventRecorderLogger
	svcOpCreator SvcOpCreator
}

func NewOperationHandler(client client.Client, svcOpCreator SvcOpCreator, recorder eventrecord.EventRecorderLogger) *OperationHandler {
	return &OperationHandler{
		client:       client,
		svcOpCreator: svcOpCreator,
		recorder:     recorder,
	}
}

// Handle triggers operation depending on conditions set by lifecycle handler.
func (h OperationHandler) Handle(ctx context.Context, vmop *virtv2.VirtualMachineOperation) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(operationHandlerName))

	if vmop == nil {
		return reconcile.Result{}, nil
	}

	// Ignore if vmop in deletion state.
	if vmop.DeletionTimestamp != nil {
		log.Debug("Skip operation, VMOP is in deletion state")
		return reconcile.Result{}, nil
	}

	// Do not perform operation if vmop not in the Pending phase.
	if vmop.Status.Phase != virtv2.VMOPPhasePending {
		log.Debug("Skip operation, VMOP is not in the Pending phase", "vmop.phase", vmop.Status.Phase)
		return reconcile.Result{}, nil
	}

	// VirtualMachineOperation should contain Complete condition in Unknown state to perform operation.
	// Other statuses may indicate waiting state, e.g. non-existent VM or other VMOPs in progress.
	completeCondition, found := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	if !found {
		log.Debug("Skip operation, no Complete condition found", "vmop.phase", vmop.Status.Phase)
		return reconcile.Result{}, nil
	}
	if completeCondition.Status != metav1.ConditionUnknown {
		log.Debug("Skip operation, Complete condition is not Unknown", "vmop.complete.status", completeCondition.Status, "vmop.phase", vmop.Status.Phase)
		return reconcile.Result{}, nil
	}

	completedCond := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).
		Generation(vmop.GetGeneration())
	signalSendCond := conditions.NewConditionBuilder(vmopcondition.SignalSentType).
		Generation(vmop.GetGeneration())

	// Send signal to perform operation, set phase to InProgress on success and to Fail on error.
	h.recordEventForVM(ctx, vmop)
	svcOp, err := h.svcOpCreator(vmop)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = svcOp.Do(ctx)
	if err != nil {
		failMsg := fmt.Sprintf("Sending signal %q to VM", vmop.Spec.Type)
		log.Debug(failMsg, logger.SlogErr(err))
		failMsg = fmt.Sprintf("%s: %v", failMsg, err)
		h.recorder.Event(vmop, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, failMsg)

		vmop.Status.Phase = virtv2.VMOPPhaseFailed
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

		return reconcile.Result{}, nil
	}

	msg := fmt.Sprintf("Sent signal %q to VM without errors.", vmop.Spec.Type)
	log.Debug(msg)
	h.recorder.Event(vmop, corev1.EventTypeNormal, virtv2.ReasonVMOPInProgress, msg)

	vmop.Status.Phase = virtv2.VMOPPhaseInProgress

	reason, err := svcOp.GetInProgressReason(ctx)
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

	// No requeue, just wait for the VM phase change.
	return reconcile.Result{}, err
}

func (h OperationHandler) Name() string {
	return operationHandlerName
}

func (h OperationHandler) recordEventForVM(ctx context.Context, vmop *virtv2.VirtualMachineOperation) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(operationHandlerName))

	if vmop == nil {
		return
	}

	// Get VM for Pending and InProgress checks.
	vm, err := object.FetchObject(ctx, types.NamespacedName{Name: vmop.Spec.VirtualMachine, Namespace: vmop.Namespace}, h.client, &virtv2.VirtualMachine{})
	if err != nil {
		// Only log the error.
		log.Error("Get VirtualMachine to record Event for VMOP", logger.SlogErr(err))
		return
	}
	if vm == nil {
		return
	}

	switch vmop.Spec.Type {
	case virtv2.VMOPTypeStart:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			virtv2.ReasonVMStarted,
			"Start initiated with VirtualMachineOperation",
		)
	case virtv2.VMOPTypeStop:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			virtv2.ReasonVMStopped,
			"Stop initiated with VirtualMachineOperation",
		)
	case virtv2.VMOPTypeRestart:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			virtv2.ReasonVMRestarted,
			"Restart initiated with VirtualMachineOperation",
		)
	case virtv2.VMOPTypeEvict:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			virtv2.ReasonVMEvicted,
			"Evict initiated with VirtualMachineOperation",
		)
	case virtv2.VMOPTypeMigrate:
		h.recorder.WithLogging(log).Event(
			vm,
			corev1.EventTypeNormal,
			virtv2.ReasonVMMigrated,
			"Migrate initiated with VirtualMachineOperation",
		)
	}
}
