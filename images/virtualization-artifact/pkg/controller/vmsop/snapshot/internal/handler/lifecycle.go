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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonvmsop "github.com/deckhouse/virtualization-controller/pkg/common/vmsop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	genericservice "github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/snapshot/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

type Base interface {
	Init(vmsop *v1alpha2.VirtualMachineSnapshotOperation)
	ShouldExecuteOrSetFailedPhase(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (bool, error)
	FetchVirtualMachineSnapshotOrSetFailedPhase(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (*v1alpha2.VirtualMachineSnapshot, error)
	IsApplicableOrSetFailedPhase(checker genericservice.ApplicableChecker, vmsop *v1alpha2.VirtualMachineSnapshotOperation, vms *v1alpha2.VirtualMachineSnapshot) bool
}

type LifecycleHandler struct {
	svcOpCreator SvcOpCreator
	base         Base
	recorder     eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(svcOpCreator SvcOpCreator, base Base, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		svcOpCreator: svcOpCreator,
		base:         base,
		recorder:     recorder,
	}
}

// Handle sets conditions depending on cluster state.
// It should set Running condition to start operation on VM.
func (h LifecycleHandler) Handle(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (reconcile.Result, error) {
	// Do not update conditions for object in the deletion state.
	if commonvmsop.IsTerminating(vmsop) {
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	svcOp, err := h.svcOpCreator(vmsop)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Ignore if VMSOP is in final state.
	if complete, _ := svcOp.IsComplete(); complete {
		return reconcile.Result{}, nil
	}

	// 1.Initialize new VMSOP resource: set phase to Pending and all conditions to Unknown.
	h.base.Init(vmsop)

	// 2. Get VirtualMachine for validation vmsop.
	vm, err := h.base.FetchVirtualMachineSnapshotOrSetFailedPhase(ctx, vmsop)
	if vm == nil || err != nil {
		return reconcile.Result{}, err
	}

	// 3. Operation already in progress. Check if the operation is completed.
	// Run execute until the operation is completed.
	if svcOp.IsInProgress() {
		return h.execute(ctx, vmsop, svcOp)
	}

	// 4. VMSOP is not in progress.
	// All operations must be performed in course, check it and set phase if operation cannot be executed now.
	should, err := h.base.ShouldExecuteOrSetFailedPhase(ctx, vmsop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !should {
		return reconcile.Result{}, nil
	}

	// 5. Check if the operation is applicable for executed.
	isApplicable := h.base.IsApplicableOrSetFailedPhase(svcOp, vmsop, vm)
	if !isApplicable {
		return reconcile.Result{}, nil
	}

	// 6. The Operation is valid, and can be executed.
	return h.execute(ctx, vmsop, svcOp)
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

func (h LifecycleHandler) execute(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation, svcOp service.Operation) (rec reconcile.Result, err error) {
	log := logger.FromContext(ctx)

	completedCond := conditions.NewConditionBuilder(vmsopcondition.TypeCompleted).Generation(vmsop.GetGeneration())
	rec, err = svcOp.Execute(ctx)
	if err != nil {
		failMsg := fmt.Sprintf("%s is failed", vmsop.Spec.Type)
		log.Debug(failMsg, logger.SlogErr(err))
		failMsg = fmt.Errorf("%s: %w", failMsg, err).Error()
		h.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, failMsg)

		vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
		conditions.SetCondition(completedCond.Reason(vmsopcondition.ReasonOperationFailed).Message(failMsg).Status(metav1.ConditionFalse), &vmsop.Status.Conditions)
	} else {
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseInProgress
		reason := svcOp.GetInProgressReason()
		conditions.SetCondition(completedCond.Reason(reason).Message("Wait for operation to complete.").Status(metav1.ConditionFalse), &vmsop.Status.Conditions)
	}

	isComplete, failMsg := svcOp.IsComplete()
	if isComplete {
		if failMsg != "" {
			vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
			h.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, failMsg)

			conditions.SetCondition(completedCond.Reason(vmsopcondition.ReasonOperationFailed).Message(failMsg).Status(metav1.ConditionFalse), &vmsop.Status.Conditions)
		} else {
			vmsop.Status.Phase = v1alpha2.VMSOPPhaseCompleted
			h.recorder.Event(vmsop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded, "VirtualMachineSnapshotOperation completed")

			conditions.SetCondition(completedCond.Reason(vmsopcondition.ReasonOperationCompleted).Message("VirtualMachineSnapshotOperation succeeded.").Status(metav1.ConditionTrue), &vmsop.Status.Conditions)
		}
	}

	return rec, err
}
