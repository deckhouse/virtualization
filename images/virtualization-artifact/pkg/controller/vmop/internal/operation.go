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
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

const operationHandlerName = "OperationHandler"

// OperationHandler performs operation on Virtual Machine.
type OperationHandler struct {
	logger   *slog.Logger
	recorder record.EventRecorder
	vmopSrv  service.VMOperationService
}

func NewOperationHandler(logger *slog.Logger, recorder record.EventRecorder, vmopSrv service.VMOperationService) *OperationHandler {
	return &OperationHandler{
		logger:   logger,
		recorder: recorder,
		vmopSrv:  vmopSrv,
	}
}

// Handle triggers operation depending on conditions set by lifecycle handler.
func (h OperationHandler) Handle(ctx context.Context, s state.VMOperationState) (reconcile.Result, error) {
	vmop := s.VirtualMachineOperation()
	if vmop == nil {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachineOperation().Changed()
	// Ignore if vmop in deletion state.
	if changed.DeletionTimestamp == nil {
		return reconcile.Result{}, nil
	}

	// Do not perform operation if vmop not in the Pending phase.
	if changed.Status.Phase != virtv2.VMOPPhasePending {
		return reconcile.Result{}, nil
	}

	// VirtualMachineOperation should contain Complete condition in Unknown state to perform operation.
	// Other statuses may indicate waiting state, e.g. non-existent VM or other VMOPs in progress.
	completeCondition, found := service.GetCondition(vmopcondition.CompletedType, changed.Status.Conditions)
	if !found {
		return reconcile.Result{}, nil
	}
	if completeCondition.Status != metav1.ConditionUnknown {
		return reconcile.Result{}, nil
	}

	// Send signal to perform operation, set phase to InProgress on success and to Fail on error.
	err := h.vmopSrv.Do(ctx, changed)
	if err != nil {
		failMsg := fmt.Sprintf("Sending powerstate signal %q to VM", changed.Spec.Type)
		h.logger.Error(failMsg, "err", err, "vmop.name", changed.GetName(), "vmop.namespace", changed.GetNamespace())
		failMsg = fmt.Sprintf("%s: %v", failMsg, err)
		h.recorder.Event(changed, corev1.EventTypeWarning, virtv2.ReasonErrVMOPFailed, failMsg)

		changed.Status.Phase = virtv2.VMOPPhaseFailed
		service.SetCondition(metav1.Condition{
			Type:   vmopcondition.SignalSentType,
			Status: metav1.ConditionFalse,
		}, &changed.Status.Conditions)
		service.SetCondition(metav1.Condition{
			Type:    vmopcondition.CompletedType,
			Status:  metav1.ConditionFalse,
			Reason:  vmopcondition.OperationFailed,
			Message: failMsg,
		}, &changed.Status.Conditions)

		return reconcile.Result{}, nil
	}

	msg := fmt.Sprintf("Sent powerstate signal %q to VM without errors.", changed.Spec.Type)
	h.logger.Debug(msg, "vmop.name", changed.GetName(), "vmop.namespace", changed.GetNamespace())
	h.recorder.Event(changed, corev1.EventTypeNormal, virtv2.ReasonVMOPSucceeded, msg)

	changed.Status.Phase = virtv2.VMOPPhaseInProgress

	service.SetCondition(metav1.Condition{
		Type:   vmopcondition.SignalSentType,
		Status: metav1.ConditionTrue,
	}, &changed.Status.Conditions)
	service.SetCondition(metav1.Condition{
		Type:    vmopcondition.CompletedType,
		Status:  metav1.ConditionFalse,
		Reason:  h.vmopSrv.InProgressReasonForType(changed),
		Message: "Wait for operation to complete",
	}, &changed.Status.Conditions)

	// No requeue, just wait for the VM phase change.
	return reconcile.Result{}, nil
}

func (h OperationHandler) Name() string {
	return operationHandlerName
}
