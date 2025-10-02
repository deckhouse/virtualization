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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

type VirtualMachineSnapshotReadyToUseHandler struct {
	client client.Client
}

func NewVirtualMachineSnapshotReadyToUseHandler(client client.Client) *VirtualMachineSnapshotReadyToUseHandler {
	return &VirtualMachineSnapshotReadyToUseHandler{
		client: client,
	}
}

func (h VirtualMachineSnapshotReadyToUseHandler) Handle(ctx context.Context, vmRestore *v1alpha2.VirtualMachineRestore) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmrestorecondition.VirtualMachineSnapshotReadyToUseType)
	defer func() { conditions.SetCondition(cb.Generation(vmRestore.Generation), &vmRestore.Status.Conditions) }()

	if !conditions.HasCondition(cb.GetType(), vmRestore.Status.Conditions) {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
	}

	if vmRestore.DeletionTimestamp != nil {
		cb.Status(metav1.ConditionUnknown).Reason(conditions.ReasonUnknown)
		return reconcile.Result{}, nil
	}

	vmSnapshotKey := types.NamespacedName{Name: vmRestore.Spec.VirtualMachineSnapshotName, Namespace: vmRestore.Namespace}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, h.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmSnapshot == nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotFound).
			Message(fmt.Sprintf("The virtual machine snapshot %q not found.", vmRestore.Spec.VirtualMachineSnapshotName))
		return reconcile.Result{}, nil
	}

	if vmSnapshot.GetDeletionTimestamp() != nil {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReady).
			Message(fmt.Sprintf("The virtual machine snapshot %q is in the process if deleting.", vmSnapshot.Name))
		return reconcile.Result{}, nil
	}

	vmSnapshotReady, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
	if vmSnapshotReady.Status != metav1.ConditionTrue || vmSnapshot.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseReady {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReady).
			Message(fmt.Sprintf("Waiting for the virtual machine snapshot %q to be ready for use.", vmSnapshot.Name))
		return reconcile.Result{}, nil
	}

	if vmSnapshot.Generation != vmSnapshot.Status.ObservedGeneration {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReady).
			Message(fmt.Sprintf("Waiting for the virtual machine snapshot %q to be observed in its latest state generation.", vmSnapshot.Name))
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionTrue).Reason(vmrestorecondition.VirtualMachineSnapshotReadyToUse)

	return reconcile.Result{}, nil
}
