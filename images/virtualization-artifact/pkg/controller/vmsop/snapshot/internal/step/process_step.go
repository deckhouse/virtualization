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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/snapshot/internal/common"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
)

type ProcessCloneStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
}

func NewProcessStep(
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

func (s ProcessCloneStep) Take(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) (*reconcile.Result, error) {
	c, exist := conditions.GetCondition(s.cb.GetType(), vmsop.Status.Conditions)
	if exist {
		if c.Status == metav1.ConditionTrue {
			return &reconcile.Result{}, nil
		}
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmsop.Namespace, Name: vmsop.Spec.VirtualMachineSnapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		common.SetPhaseCreateVirtualMachineConditionToFailed(s.cb, &vmsop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	restorerSecret, err := object.FetchObject(ctx, restorerSecretKey, s.client, &corev1.Secret{})
	if err != nil {
		common.SetPhaseCreateVirtualMachineConditionToFailed(s.cb, &vmsop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	snapshotResources := restorer.NewSnapshotResources(s.client, v1alpha2.VMOPTypeClone, v1alpha2.VMOPRestoreModeStrict, restorerSecret, vmSnapshot, string(vmsop.UID))

	err = snapshotResources.Prepare(ctx)
	if err != nil {
		common.SetPhaseCreateVirtualMachineConditionToFailed(s.cb, &vmsop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	snapshotResources.Override(vmsop.Spec.CreateVirtualMachine.NameReplacement)

	if vmsop.Spec.CreateVirtualMachine.Customization != nil {
		snapshotResources.Customize(
			vmsop.Spec.CreateVirtualMachine.Customization.NamePrefix,
			vmsop.Spec.CreateVirtualMachine.Customization.NameSuffix,
		)
	}

	statuses, err := snapshotResources.Validate(ctx)
	vmsop.Status.Resources = statuses
	if err != nil {
		s.cb.Status(metav1.ConditionFalse).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationFailed).Message(service.CapitalizeFirstLetter(err.Error()))
		return &reconcile.Result{}, nil
	}

	if vmsop.Spec.CreateVirtualMachine.Mode == v1alpha2.VMSOPCreateVirtualMachineModeDryRun {
		s.cb.Status(metav1.ConditionTrue).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationCompleted).Message("The virtual machine can be cloned from the snapshot.")
		return &reconcile.Result{}, nil
	}

	statuses, err = snapshotResources.Process(ctx)
	vmsop.Status.Resources = statuses
	if err != nil {
		common.SetPhaseCreateVirtualMachineConditionToFailed(s.cb, &vmsop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	for _, status := range vmsop.Status.Resources {
		if status.Kind != v1alpha2.VirtualDiskKind {
			continue
		}

		var vd v1alpha2.VirtualDisk
		vdKey := types.NamespacedName{Namespace: vmsop.Namespace, Name: status.Name}
		err := s.client.Get(ctx, vdKey, &vd)
		if err != nil {
			return &reconcile.Result{}, fmt.Errorf("failed to get the `VirtualDisk`: %w", err)
		}

		if vd.Annotations[annotations.AnnVMOPRestore] != string(vmsop.UID) {
			return &reconcile.Result{}, nil
		}

		if vd.Status.Phase == v1alpha2.DiskFailed {
			common.SetPhaseCreateVirtualMachineConditionToFailed(s.cb, &vmsop.Status.Phase, err)
			return &reconcile.Result{}, fmt.Errorf("virtual disk %q is in failed phase", vdKey.Name)
		}

		if vd.Status.Phase != v1alpha2.DiskReady {
			return &reconcile.Result{}, nil
		}
	}

	s.cb.Status(metav1.ConditionTrue).Reason(vmsopcondition.ReasonCreateVirtualMachineOperationCompleted).Message("The virtual machine has been cloned successfully.")

	return &reconcile.Result{}, nil
}
