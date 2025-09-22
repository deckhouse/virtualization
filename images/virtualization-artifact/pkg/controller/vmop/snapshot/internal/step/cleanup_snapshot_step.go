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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type CleanupSnapshotStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewCleanupSnapshotStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *CleanupSnapshotStep {
	return &CleanupSnapshotStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
	}
}

func (s CleanupSnapshotStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	rcb := conditions.NewConditionBuilder(vmopcondition.TypeSnapshotReady)

	snapshotCondition, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
	if found && snapshotCondition.Reason == string(vmopcondition.ReasonSnapshotCleanedUp) {
		return nil, nil
	}

	cloneCondition, found := conditions.GetCondition(vmopcondition.TypeCloneCompleted, vmop.Status.Conditions)
	if !found || cloneCondition.Reason == string(vmopcondition.ReasonCloneOperationInProgress) || cloneCondition.Status == metav1.ConditionUnknown {
		return nil, nil
	}

	if cloneCondition.Reason != string(vmopcondition.ReasonCloneOperationFailed) && cloneCondition.Status == metav1.ConditionFalse {
		return nil, nil
	}

	for _, status := range vmop.Status.Resources {
		if status.Status == v1alpha2.VMOPResourceStatusInProgress {
			return nil, nil
		}
	}

	snapshotName, ok := vmop.Annotations[annotations.AnnVMOPSnapshotName]
	if !ok {
		return nil, nil
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmop.Namespace, Name: snapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		return &reconcile.Result{}, err
	}

	if vmSnapshot == nil {
		conditions.SetCondition(
			rcb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonSnapshotCleanedUp).Message("Snapshot cleanup completed."),
			&vmop.Status.Conditions,
		)
		return &reconcile.Result{}, nil
	}

	if !object.IsTerminating(vmSnapshot) {
		err := s.client.Delete(ctx, vmSnapshot)
		if err != nil && !apierrors.IsNotFound(err) {
			return &reconcile.Result{}, fmt.Errorf("failed to delete the `VirtualMachineSnapshot`: %w", err)
		}

		s.recorder.Event(vmop, corev1.EventTypeNormal, "SnapshotDeleted", fmt.Sprintf("Deleted snapshot %s after clone completion", vmSnapshot.Name))
	}

	conditions.SetCondition(
		rcb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonSnapshotCleanedUp).Message("Snapshot cleanup completed."),
		&vmop.Status.Conditions,
	)

	return &reconcile.Result{}, nil
}
