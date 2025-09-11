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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/snapshot/internal/common"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type ProcessCloneStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewProcessCloneStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *ProcessCloneStep {
	return &ProcessCloneStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
	}
}

func (s ProcessCloneStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	c, exist := conditions.GetCondition(s.cb.GetType(), vmop.Status.Conditions)
	if exist {
		if c.Status == metav1.ConditionTrue {
			return &reconcile.Result{}, nil
		}

		snapshotReadyCondition, found := conditions.GetCondition(vmopcondition.TypeSnapshotReady, vmop.Status.Conditions)
		if found && snapshotReadyCondition.Status == metav1.ConditionFalse {
			return &reconcile.Result{}, nil
		}
	}

	snapshotName, ok := vmop.Annotations[annotations.AnnVMOPSnapshotName]
	if !ok {
		err := fmt.Errorf("snapshot name annotation not found")
		common.SetPhaseCloneConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmop.Namespace, Name: snapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		common.SetPhaseCloneConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, s.client, &corev1.Secret{})
	if err != nil {
		common.SetPhaseCloneConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	snapshotResources := restorer.NewSnapshotResources(s.client, v1alpha2.VMOPTypeClone, v1alpha2.VMOPRestoreModeStrict, restorerSecret, vmSnapshot, string(vmop.UID))

	err = snapshotResources.Prepare(ctx)
	if err != nil {
		common.SetPhaseCloneConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	snapshotResources.Override(vmop.Spec.Clone.NameReplacement)

	if vmop.Spec.Clone.Customization != nil {
		snapshotResources.Customize(
			vmop.Spec.Clone.Customization.NamePrefix,
			vmop.Spec.Clone.Customization.NameSuffix,
		)
	}

	statuses, err := snapshotResources.Validate(ctx)
	common.FillResourcesStatuses(vmop, statuses)
	if err != nil {
		s.cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonCloneOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return &reconcile.Result{}, nil
	}

	if vmop.Spec.Clone.Mode == v1alpha2.VMOPRestoreModeDryRun {
		s.cb.Status(metav1.ConditionTrue).Reason(vmopcondition.ReasonCloneOperationCompleted).Message("The virtual machine can be cloned from the snapshot.")
		return &reconcile.Result{}, nil
	}

	statuses, err = snapshotResources.Process(ctx)
	common.FillResourcesStatuses(vmop, statuses)
	if err != nil {
		common.SetPhaseCloneConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	for _, status := range vmop.Status.Resources {
		if status.Kind != v1alpha2.VirtualDiskKind {
			continue
		}

		var vd v1alpha2.VirtualDisk
		vdKey := types.NamespacedName{Namespace: vmop.Namespace, Name: status.Name}
		err := s.client.Get(ctx, vdKey, &vd)
		if err != nil {
			return &reconcile.Result{}, fmt.Errorf("failed to get the `VirtualDisk`: %w", err)
		}

		if vd.Annotations[annotations.AnnVMOPRestore] != string(vmop.UID) {
			return &reconcile.Result{}, nil
		}

		if vd.Status.Phase == v1alpha2.DiskFailed {
			conditions.SetCondition(
				s.cb.Status(metav1.ConditionFalse).Reason(vmopcondition.ReasonSnapshotFailed).Message("Snapshot is failed."),
				&vmop.Status.Conditions,
			)
			common.SetPhaseCloneConditionToFailed(s.cb, &vmop.Status.Phase, err)
			return &reconcile.Result{}, fmt.Errorf("virtual disk %q is in failed phase", vdKey.Name)
		}

		if vd.Status.Phase != v1alpha2.DiskReady {
			return &reconcile.Result{}, nil
		}
	}

	s.cb.Status(metav1.ConditionTrue).Reason(vmopcondition.ReasonCloneOperationCompleted).Message("The virtual machine has been cloned successfully.")

	return &reconcile.Result{}, nil
}
