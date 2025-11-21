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

func (h LifecycleHandler) Handle(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmsopcondition.TypeCompleted).Generation(vmsop.GetGeneration())

	if !vmsop.GetDeletionTimestamp().IsZero() {
		vmsop.Status.Phase = v1alpha2.VMSOPPhaseTerminating
		return reconcile.Result{}, nil
	}

	cond, _ := conditions.GetCondition(vmsopcondition.TypeCompleted, vmsop.Status.Conditions)
	if cond.Reason == string(vmsopcondition.ReasonOperationCompleted) || cond.Reason == string(vmsopcondition.ReasonOperationFailed) {
		return reconcile.Result{}, nil
	}

	if vmsop.Spec.CreateVirtualMachine == nil {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonOperationFailed, "clone specification is mandatory to start creating virtual machine")
		return reconcile.Result{}, nil
	}

	vmsop.Status.Phase = v1alpha2.VMSOPPhasePending
	h.recorder.Event(vmsop, corev1.EventTypeNormal, v1alpha2.ReasonVMSOPStarted, "VirtualMachineSnapshotOperation started")
	conditions.SetCondition(cb.Reason(conditions.ReasonUnknown).Status(metav1.ConditionUnknown).Message(""), &vmsop.Status.Conditions)

	vms, err := object.FetchObject(ctx, types.NamespacedName{Name: vmsop.Spec.VirtualMachineSnapshotName, Namespace: vmsop.Namespace}, h.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonVirtualMachineSnapshotNotFound, "fail to fetch the virtual machine snapshot")
		return reconcile.Result{}, err
	}

	if vms == nil {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonVirtualMachineSnapshotNotFound, "virtual machine snapshot not found")
		return reconcile.Result{}, nil
	}

	if vms.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseReady {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonNotReadyToBeExecuted, "virtual machine snapshot is not ready")
		return reconcile.Result{}, nil
	}

	restorerSecretKey := types.NamespacedName{Namespace: vms.Namespace, Name: vms.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, h.client, &corev1.Secret{})
	if err != nil {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonNotReadyToBeExecuted, "fail to fetch the virtual machine snapshot secret")
		return reconcile.Result{}, err
	}

	if restorerSecret == nil {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonVirtualMachineSnapshotNotFound, "virtual machine snapshot secret is nil")
		return reconcile.Result{}, nil
	}

	hasInProgress, err := h.hasOperationsInProgress(ctx, vmsop)
	if err != nil {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonNotReadyToBeExecuted, "fail to check if there is any operation in progress")
		return reconcile.Result{}, err
	}
	if hasInProgress {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonNotReadyToBeExecuted, "VMSOP cannot be executed now. Previously created operation should finish first")
		return reconcile.Result{}, nil
	}

	err = h.createOp.Execute(ctx, vmsop, vms, restorerSecret)
	if err != nil {
		h.setFailedCondition(cb, vmsop, vmsopcondition.ReasonOperationFailed, fmt.Errorf("%s is failed: %w", vmsop.Spec.Type, err).Error())
	} else {
		msg := "VirtualMachineSnapshotOperation completed"
		if vmsop.Spec.CreateVirtualMachine.Mode == v1alpha2.SnapshotOperationModeDryRun {
			msg += ". The virtual machine can be cloned from the snapshot"
		}

		h.setCompletedCondition(cb, vmsop, vmsopcondition.ReasonOperationCompleted, msg)
	}

	return reconcile.Result{}, nil
}

func (h LifecycleHandler) Name() string {
	return lifecycleHandlerName
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

func (h *LifecycleHandler) setCompletedCondition(cb *conditions.ConditionBuilder, vmsop *v1alpha2.VirtualMachineSnapshotOperation, reason vmsopcondition.ReasonCompleted, message string) {
	vmsop.Status.Phase = v1alpha2.VMSOPPhaseCompleted
	conditions.SetCondition(cb.Reason(reason).Message(message).Status(metav1.ConditionTrue), &vmsop.Status.Conditions)
	h.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonVMOPSucceeded, message)
}

func (h *LifecycleHandler) setFailedCondition(cb *conditions.ConditionBuilder, vmsop *v1alpha2.VirtualMachineSnapshotOperation, reason vmsopcondition.ReasonCompleted, message string) {
	vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
	conditions.SetCondition(cb.Reason(reason).Message(message).Status(metav1.ConditionFalse), &vmsop.Status.Conditions)
	h.recorder.Event(vmsop, corev1.EventTypeWarning, v1alpha2.ReasonErrVMSOPFailed, message)
}
