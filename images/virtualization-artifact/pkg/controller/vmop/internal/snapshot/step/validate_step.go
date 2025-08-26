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

package step

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
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	restorercommon "github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/snapshot/common"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type ValidateStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
	vmop     *virtv2.VirtualMachineOperation
}

func NewValidateStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	vmop *virtv2.VirtualMachineOperation,
) *ValidateStep {
	return &ValidateStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
		vmop:     vmop,
	}
}

func (s ValidateStep) Take(ctx context.Context, vm *virtv2.VirtualMachine) (*reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmopcondition.TypeRestoreCompleted)
	defer func() { conditions.SetCondition(cb.Generation(s.vmop.Generation), &s.vmop.Status.Conditions) }()

	if conditions.HasCondition(cb.GetType(), s.vmop.Status.Conditions) && cb.Condition().Status == metav1.ConditionTrue {
		return nil, nil
	}

	vmSnapshotKey := types.NamespacedName{Namespace: s.vmop.Namespace, Name: s.vmop.Spec.Restore.VirtualMachineSnapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &virtv2.VirtualMachineSnapshot{})
	if err != nil {
		common.SetPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	if vmSnapshot.Status.VirtualMachineSnapshotSecretName == "" {
		err := fmt.Errorf("snapshot secret name is empty")
		common.SetPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, s.client, &corev1.Secret{})
	if err != nil {
		common.SetPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	snapshotResources := restorer.NewSnapshotResources(s.client, restorercommon.RestoreKind, restorercommon.DryRunMode, restorerSecret, vmSnapshot, string(s.vmop.UID))

	err = snapshotResources.Prepare(ctx)
	if err != nil {
		common.SetPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	statuses, err := snapshotResources.Validate(ctx)
	common.FillResourcesStatuses(s.vmop, statuses)
	if err != nil {
		common.SetPhaseConditionToFailed(cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	// if not DryRun continue restore
	if s.vmop.Spec.Restore.Mode != virtv2.VMOPRestoreModeDryRun {
		return nil, nil
	}

	common.SetPhaseConditionCompleted(cb, &s.vmop.Status.Phase, vmopcondition.ReasonRestoreOperationCompleted, "The virtual machine can be restored from the snapshot")

	return &reconcile.Result{}, nil
}
