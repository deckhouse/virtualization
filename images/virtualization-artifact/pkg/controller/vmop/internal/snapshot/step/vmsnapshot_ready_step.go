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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	vmrestorecondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vm-restore-condition"
)

type VMSnapshotReadyStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
	cb       *conditions.ConditionBuilder
	vmop     *virtv2.VirtualMachineOperation
}

func NewVMSnapshotReadyStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	vmop *virtv2.VirtualMachineOperation,
) *VMSnapshotReadyStep {
	return &VMSnapshotReadyStep{
		client:   client,
		recorder: recorder,
		cb:       cb,
		vmop:     vmop,
	}
}

func (s VMSnapshotReadyStep) Take(ctx context.Context, vm *virtv2.VirtualMachine) (*reconcile.Result, error) {
	if s.vmop.Spec.Restore.VirtualMachineSnapshotName == "" {
		err := fmt.Errorf("the virtual machine snapshot name is empty")
		setPhaseConditionToFailed(s.cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	vmSnapshotKey := types.NamespacedName{Namespace: s.vmop.Namespace, Name: s.vmop.Spec.Restore.VirtualMachineSnapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &virtv2.VirtualMachineSnapshot{})
	if err != nil {
		setPhaseConditionToFailed(s.cb, &s.vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	vmSnapshotReadyToUseCondition, _ := conditions.GetCondition(vmrestorecondition.VirtualMachineSnapshotReadyToUseType, vmSnapshot.Status.Conditions)
	if vmSnapshotReadyToUseCondition.Status != metav1.ConditionTrue {
		s.vmop.Status.Phase = virtv2.VMOPPhasePending
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmrestorecondition.VirtualMachineSnapshotNotReadyToUse).
			Message(fmt.Sprintf("Waiting for the virtual machine snapshot %q to be ready to use.", s.vmop.Spec.Restore.VirtualMachineSnapshotName))
		return &reconcile.Result{}, nil
	}

	return &reconcile.Result{}, nil
}
