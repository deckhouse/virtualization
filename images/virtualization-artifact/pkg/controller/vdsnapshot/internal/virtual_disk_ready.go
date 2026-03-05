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
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type VirtualDiskReadyHandler struct {
	snapshotter VirtualDiskReadySnapshotter
	client      client.Client
}

func NewVirtualDiskReadyHandler(snapshotter VirtualDiskReadySnapshotter, client client.Client) *VirtualDiskReadyHandler {
	return &VirtualDiskReadyHandler{
		snapshotter: snapshotter,
		client:      client,
	}
}

func (h VirtualDiskReadyHandler) Handle(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vdscondition.VirtualDiskReadyType).Generation(vdSnapshot.Generation)

	defer func() { conditions.SetCondition(cb, &vdSnapshot.Status.Conditions) }()

	if vdSnapshot.DeletionTimestamp != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")
		return reconcile.Result{}, nil
	}

	if vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseReady {
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdscondition.VirtualDiskReady).
			Message("")
		return reconcile.Result{}, nil
	}

	vd, err := h.snapshotter.GetVirtualDisk(ctx, vdSnapshot.Spec.VirtualDiskName, vdSnapshot.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vd == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.VirtualDiskNotReadyForSnapshotting).
			Message(fmt.Sprintf("The virtual disk %q not found.", vdSnapshot.Spec.VirtualDiskName))
		return reconcile.Result{}, nil
	}

	if vd.GetDeletionTimestamp() != nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.VirtualDiskNotReadyForSnapshotting).
			Message(fmt.Sprintf("The virtual disk %q is in process of deletion.", vd.Name))
		return reconcile.Result{}, nil
	}

	switch vd.Status.Phase {
	case v1alpha2.DiskReady:
		snapshotting, ok := conditions.GetCondition(vdcondition.SnapshottingType, vd.Status.Conditions)
		// If the snapshotting condition is not found, it means that the disk is ready for snapshotting.
		// Otherwise, check the status of the condition and ensure it reflects the current state of the object.
		if ok && (snapshotting.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(snapshotting, vd)) {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdscondition.VirtualDiskNotReadyForSnapshotting).
				Message(snapshotting.Message)
			return reconcile.Result{}, nil
		}

		attachedToMigratingVM, err := h.checkDiskNotAttachedToMigratingVM(ctx, vd, cb)
		if err != nil {
			return reconcile.Result{}, err
		}
		if attachedToMigratingVM {
			return reconcile.Result{}, nil
		}

		cb.
			Status(metav1.ConditionTrue).
			Reason(vdscondition.VirtualDiskReady).
			Message("")
		return reconcile.Result{}, nil
	default:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdscondition.VirtualDiskNotReadyForSnapshotting).
			Message(fmt.Sprintf("The virtual disk %q is in the %q phase: waiting for it to reach the Ready phase.", vd.Name, vd.Status.Phase))
		return reconcile.Result{}, nil
	}
}

// checkDiskNotAttachedToMigratingVM checks that the disk is not attached to a migrating VM.
// Returns (true, nil) if the disk is attached to a migrating VM (caller should return immediately).
// Otherwise returns (false, nil) or (false, err).
func (h VirtualDiskReadyHandler) checkDiskNotAttachedToMigratingVM(ctx context.Context, vd *v1alpha2.VirtualDisk, cb *conditions.ConditionBuilder) (bool, error) {
	inUse, _ := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
	if inUse.Status != metav1.ConditionTrue {
		return false, nil
	}

	if len(vd.Status.AttachedToVirtualMachines) != 1 {
		return false, errors.New("disk is in InUse state but the number of VMs it is attached to is not 1; this should not happen, please report a bug")
	}

	vmName := vd.Status.AttachedToVirtualMachines[0].Name
	vm, err := object.FetchObject(ctx, types.NamespacedName{Name: vmName, Namespace: vd.Namespace}, h.client, &v1alpha2.VirtualMachine{})
	if err != nil {
		return false, err
	}

	_, migratingConditionExists := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	if !migratingConditionExists {
		return false, nil
	}
	cb.
		Status(metav1.ConditionFalse).
		Reason(vdscondition.VirtualDiskNotReadyForSnapshotting).
		Message("Snapshot cannot be taken: the virtual machine is currently migrating.")
	return true, nil
}
