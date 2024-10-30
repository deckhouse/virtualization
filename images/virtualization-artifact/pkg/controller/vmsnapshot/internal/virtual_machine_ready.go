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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

type VirtualMachineReadySnapshotter interface {
	GetVirtualMachine(ctx context.Context, name, namespace string) (*virtv2.VirtualMachine, error)
}

type VirtualMachineReadyHandler struct {
	snapshotter VirtualMachineReadySnapshotter
}

func NewVirtualMachineReadyHandler(snapshotter VirtualMachineReadySnapshotter) *VirtualMachineReadyHandler {
	return &VirtualMachineReadyHandler{
		snapshotter: snapshotter,
	}
}

func (h VirtualMachineReadyHandler) Handle(ctx context.Context, vmSnapshot *virtv2.VirtualMachineSnapshot) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmscondition.VirtualMachineReadyType)
	defer func() { conditions.SetCondition(cb.Generation(vmSnapshot.Generation), &vmSnapshot.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmSnapshot.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(vmscondition.VirtualMachineUnknown)
	}

	if vmSnapshot.DeletionTimestamp != nil {
		cb.Status(metav1.ConditionUnknown).Reason(vmscondition.VirtualMachineUnknown)
		return reconcile.Result{}, nil
	}

	if vmSnapshot.Status.Phase == virtv2.VirtualMachineSnapshotPhaseReady {
		cb.Status(metav1.ConditionTrue).Reason(vmscondition.VirtualMachineReady)
		return reconcile.Result{}, nil
	}

	vm, err := h.snapshotter.GetVirtualMachine(ctx, vmSnapshot.Spec.VirtualMachineName, vmSnapshot.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vm == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.VirtualMachineNotReadyForSnapshotting).
			Message(fmt.Sprintf("The virtual machine %q not found.", vmSnapshot.Spec.VirtualMachineName))
		return reconcile.Result{}, nil
	}

	if vm.GetDeletionTimestamp() != nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.VirtualMachineNotReadyForSnapshotting).
			Message(fmt.Sprintf("The virtual machine %q is in process of deletion.", vm.Name))
		return reconcile.Result{}, nil
	}

	switch vm.Status.Phase {
	case virtv2.MachineRunning, virtv2.MachineStopped:
		snapshotting, _ := conditions.GetConditionByType(vmcondition.TypeSnapshotting.String(), vm.Status.Conditions)
		if snapshotting.Status != metav1.ConditionTrue {
			cb.Status(metav1.ConditionFalse).Reason(vmscondition.VirtualMachineNotReadyForSnapshotting)
			if snapshotting.Message == "" {
				cb.Message("The VirtualMachineSnapshot resource has not been detected for the virtual machine yet.")
			} else {
				cb.Message(snapshotting.Message)
			}

			return reconcile.Result{}, nil
		}

		cb.Status(metav1.ConditionTrue).Reason(vmscondition.VirtualMachineReady)
		return reconcile.Result{}, nil
	default:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmscondition.VirtualMachineNotReadyForSnapshotting).
			Message(fmt.Sprintf("The virtual machine %q is in the %q phase: waiting for it to reach the Running or Stopped phase.", vm.Name, vm.Status.Phase))
		return reconcile.Result{}, nil
	}
}
