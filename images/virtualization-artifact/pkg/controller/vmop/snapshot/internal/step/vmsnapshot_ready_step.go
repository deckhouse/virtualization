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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
)

type VMSnapshotReadyStep struct {
	client client.Client
}

func NewVMSnapshotReadyStep(
	client client.Client,
) *VMSnapshotReadyStep {
	return &VMSnapshotReadyStep{
		client: client,
	}
}

func (s VMSnapshotReadyStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	var vmopName string
	if vmop.Spec.Type == v1alpha2.VMOPTypeRestore {
		if vmop.Spec.Restore.VirtualMachineSnapshotName == "" {
			err := fmt.Errorf("the virtual machine snapshot name is empty")
			return &reconcile.Result{}, err
		}

		vmopName = vmop.Spec.Restore.VirtualMachineSnapshotName
	} else {
		snapshotName, exist := vmop.Annotations[annotations.AnnVMOPSnapshotName]
		if !exist {
			return &reconcile.Result{}, nil
		}
		vmopName = snapshotName
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmop.Namespace, Name: vmopName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		return &reconcile.Result{}, err
	}

	if vmSnapshot == nil {
		return &reconcile.Result{}, fmt.Errorf("virtual machine snapshot %q is not found", vmSnapshotKey.Name)
	}

	vmSnapshotReadyToUseCondition, exist := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
	if !exist {
		return &reconcile.Result{}, fmt.Errorf("virtual machine snapshot %q is not ready to use", vmopName)
	}

	if vmSnapshotReadyToUseCondition.Status != metav1.ConditionTrue {
		return &reconcile.Result{}, fmt.Errorf("virtual machine snapshot %q is not ready to use", vmopName)
	}

	if vmSnapshot.Status.VirtualMachineSnapshotSecretName == "" {
		return &reconcile.Result{}, fmt.Errorf("snapshot secret name is empty")
	}

	return nil, nil
}
