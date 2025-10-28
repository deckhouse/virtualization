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

package step

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/snapshot/internal/common"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type ProcessRestoreStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewProcessRestoreStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *ProcessRestoreStep {
	return &ProcessRestoreStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
	}
}

func (s ProcessRestoreStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	c, exist := conditions.GetCondition(s.cb.GetType(), vmop.Status.Conditions)
	if exist {
		if c.Status == metav1.ConditionTrue {
			return nil, nil
		}

		maintenanceModeCondition, found := conditions.GetCondition(vmopcondition.TypeMaintenanceMode, vmop.Status.Conditions)
		if found && maintenanceModeCondition.Status == metav1.ConditionFalse {
			return &reconcile.Result{}, nil
		}
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.Restore.VirtualMachineSnapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, s.client, &corev1.Secret{})
	if err != nil {
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	snapshotResources := restorer.NewSnapshotResources(s.client, v1alpha2.VMOPTypeRestore, vmop.Spec.Restore.Mode, restorerSecret, vmSnapshot, string(vmop.UID))

	err = snapshotResources.Prepare(ctx)
	if err != nil {
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	statuses, err := snapshotResources.Validate(ctx)
	vmop.Status.Resources = statuses
	if err != nil {
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	if vmop.Spec.Restore.Mode == v1alpha2.VMOPRestoreModeDryRun {
		s.cb.Status(metav1.ConditionTrue).Reason(vmopcondition.ReasonRestoreOperationCompleted).Message("The virtual machine can be restored from the snapshot.")
		common.SetPhaseConditionCompleted(s.cb, &vmop.Status.Phase, vmopcondition.ReasonDryRunOperationCompleted, "The virtual machine can be restored from the snapshot")
		return &reconcile.Result{}, nil
	}

	statuses, err = snapshotResources.Process(ctx)
	vmop.Status.Resources = statuses
	if err != nil {
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	for _, status := range statuses {
		if status.Status == v1alpha2.SnapshotResourceStatusInProgress {
			return &reconcile.Result{}, nil
		}
	}

	s.cb.Status(metav1.ConditionTrue).Reason(vmopcondition.ReasonRestoreOperationCompleted).Message("The virtual machine has been restored from the snapshot successfully.")

	return &reconcile.Result{}, nil
}
