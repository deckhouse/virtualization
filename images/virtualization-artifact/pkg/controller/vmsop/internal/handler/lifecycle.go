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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
)

const lifecycleHandlerName = "LifecycleHandler"

type LifecycleHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	createOp CreateOpeartioner
}

func NewLifecycleHandler(client client.Client, createOp CreateOpeartioner, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		client:   client,
		recorder: recorder,
		createOp: createOp,
	}
}

// Handle sets conditions depending on cluster state.
// It should set Running condition to start operation on VM.
func (h LifecycleHandler) Handle(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmsopcondition.TypeCompleted).Generation(vmsop.GetGeneration())

	// Do not update conditions for object in the deletion state.
	if !vmsop.GetDeletionTimestamp().IsZero() {
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	// Ignore if VMSOP is in final state.
	if complete, _ := h.createOp.IsFinished(vmsop); complete {
		return reconcile.Result{}, nil
	}

	// 1.Initialize new VMSOP resource: set phase to Pending and all conditions to Unknown.
	h.recorder.Event(vmsop, corev1.EventTypeNormal, v1alpha2.ReasonVMSOPStarted, "VirtualMachineSnapshotOperation started")
	vmsop.Status.Phase = v1alpha2.VMSOPPhasePending
	// Add all conditions in unknown state.
	conditions.SetCondition(cb.Reason(conditions.ReasonUnknown).Status(metav1.ConditionUnknown).Message(""), &vmsop.Status.Conditions)

	// 2. Get VirtualMachineSnapshot for validation vmsop.
	vms, err := object.FetchObject(ctx, types.NamespacedName{Name: vmsop.Spec.VirtualMachineSnapshotName, Namespace: vmsop.Namespace}, h.client, &v1alpha2.VirtualMachineSnapshot{})
	if vms == nil || err != nil {
		h.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, "VirtualMachineSnapshot not found")
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed

		conditions.SetCondition(
			cb.Reason(vmsopcondition.ReasonVirtualMachineSnapshotNotFound).Status(metav1.ConditionFalse).Message("VirtualMachineSnapshot not found"),
			&vmsop.Status.Conditions,
		)

		return reconcile.Result{}, nil
	}

	// 3. Operation already in progress. Check if the operation is completed.
	// Run execute until the operation is completed.
	if h.createOp.IsInProgress(vmsop) {
		return h.execute(ctx, cb, h.createOp, vmsop)
	}

	// 4. VMSOP is not in progress.
	// All operations must be performed in course, check it and set phase if operation cannot be executed now.
	hasInProgress, err := h.hasOperationsInProgress(ctx, vmsop)
	if err != nil {
		return reconcile.Result{}, err
	}
	if hasInProgress {
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
		conditions.SetCondition(
			cb.Reason(vmsopcondition.ReasonNotReadyToBeExecuted).Status(metav1.ConditionFalse).Message("VMSOP cannot be executed now. Previously created operation should finish first."),
			&vmsop.Status.Conditions,
		)

		return reconcile.Result{}, nil
	}

	// 5. Check if the operation is applicable for executed.
	if vms.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseReady {
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
		conditions.SetCondition(
			cb.Reason(vmsopcondition.ReasonNotReadyToBeExecuted).Status(metav1.ConditionFalse).Message("VMSOP cannot be executed. Snapshot is not ready."),
			&vmsop.Status.Conditions,
		)
		return reconcile.Result{}, nil
	}

	// 6. The Operation is valid, and can be executed.
	return h.execute(ctx, cb, h.createOp, vmsop)
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
}

func (h LifecycleHandler) execute(ctx context.Context, cb *conditions.ConditionBuilder, createOp CreateOpeartioner, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (rec reconcile.Result, err error) {
	rec, err = createOp.Execute(ctx, vmsop)
	if err != nil {
		failMsg := fmt.Sprintf("%s is failed", vmsop.Spec.Type)
		failMsg = fmt.Errorf("%s: %w", failMsg, err).Error()
		h.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, failMsg)

		vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
		conditions.SetCondition(cb.Reason(vmsopcondition.ReasonOperationFailed).Message(failMsg).Status(metav1.ConditionFalse), &vmsop.Status.Conditions)
	} else {
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseInProgress
		conditions.SetCondition(cb.Reason(vmsopcondition.ReasonCreateVirtualMachineInProgress).Message("Wait for operation to complete.").Status(metav1.ConditionFalse), &vmsop.Status.Conditions)
	}

	isFinished, failMsg := createOp.IsFinished(vmsop)
	if isFinished {
		if failMsg != "" {
			vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
			h.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, failMsg)
			conditions.SetCondition(cb.Reason(vmsopcondition.ReasonOperationFailed).Message(failMsg).Status(metav1.ConditionFalse), &vmsop.Status.Conditions)
			return rec, nil
		} else {
			vmsop.Status.Phase = v1alpha2.VMSOPPhaseCompleted
			h.recorder.Event(vmsop, corev1.EventTypeNormal, v1alpha2.ReasonVMOPSucceeded, "VirtualMachineSnapshotOperation completed")
			conditions.SetCondition(cb.Reason(vmsopcondition.ReasonOperationCompleted).Message("VirtualMachineSnapshotOperation succeeded.").Status(metav1.ConditionTrue), &vmsop.Status.Conditions)
		}
	}

	return rec, nil
}

func (h LifecycleHandler) hasOperationsInProgress(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (bool, error) {
	var vmsopList v1alpha2.VirtualMachineSnapshotOperationList
	err := h.client.List(ctx, &vmsopList, client.InNamespace(vmsop.GetNamespace()))
	if err != nil {
		return false, err
	}

	for _, other := range vmsopList.Items {
		if other.Spec.VirtualMachineSnapshotName != vmsop.Spec.VirtualMachineSnapshotName {
			continue
		}
		if other.Status.Phase == v1alpha2.VMSOPPhaseFailed || other.Status.Phase == v1alpha2.VMSOPPhaseCompleted {
			continue
		}
		if other.GetUID() == vmsop.GetUID() {
			continue
		}
		if other.CreationTimestamp.Before(ptr.To(vmsop.CreationTimestamp)) {
			return true, nil
		}
	}

	return false, nil
}
