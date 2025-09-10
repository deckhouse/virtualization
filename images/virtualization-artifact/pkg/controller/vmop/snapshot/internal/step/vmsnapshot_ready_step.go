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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/snapshot/internal/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

type VMSnapshotReadyStep struct {
	client client.Client
	cb     *conditions.ConditionBuilder
}

func NewVMSnapshotReadyStep(
	client client.Client,
	cb *conditions.ConditionBuilder,
) *VMSnapshotReadyStep {
	return &VMSnapshotReadyStep{
		client: client,
		cb:     cb,
	}
}

func (s VMSnapshotReadyStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	if vmop.Spec.Restore.VirtualMachineSnapshotName == "" {
		err := fmt.Errorf("the virtual machine snapshot name is empty")
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmop.Namespace, Name: vmop.Spec.Restore.VirtualMachineSnapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		err := fmt.Errorf("failed to fetch the virtual machine snapshot %q: %w", vmSnapshotKey.Name, err)
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	if vmSnapshot == nil {
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		err := fmt.Errorf("virtual machine snapshot %q is not found", vmSnapshotKey.Name)
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	vmSnapshotReadyToUseCondition, exist := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
	if !exist {
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		err := fmt.Errorf("virtual machine snapshot %q is not ready to use", vmop.Spec.Restore.VirtualMachineSnapshotName)
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	if vmSnapshotReadyToUseCondition.Status != metav1.ConditionTrue {
		vmop.Status.Phase = v1alpha2.VMOPPhaseFailed
		err := fmt.Errorf("virtual machine snapshot %q is not ready to use", vmop.Spec.Restore.VirtualMachineSnapshotName)
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	if vmSnapshot.Status.VirtualMachineSnapshotSecretName == "" {
		err := fmt.Errorf("snapshot secret name is empty")
		common.SetPhaseConditionToFailed(s.cb, &vmop.Status.Phase, err)
		return &reconcile.Result{}, err
	}

	return nil, nil
}
