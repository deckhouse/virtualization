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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type CleanupSnapshotStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func NewCleanupSnapshotStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
) *CleanupSnapshotStep {
	return &CleanupSnapshotStep{
		client:   client,
		recorder: recorder,
	}
}

func (s CleanupSnapshotStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	snapshotName, ok := vmop.Annotations[annotations.AnnVMOPSnapshotName]
	if !ok {
		return &reconcile.Result{}, nil
	}

	vmSnapshotKey := types.NamespacedName{Namespace: vmop.Namespace, Name: snapshotName}
	vmSnapshot, err := object.FetchObject(ctx, vmSnapshotKey, s.client, &v1alpha2.VirtualMachineSnapshot{})
	if err != nil {
		return &reconcile.Result{}, err
	}

	if vmSnapshot == nil {
		return &reconcile.Result{}, nil
	}

	for _, status := range vmop.Status.Resources {
		if status.Kind != v1alpha2.VirtualDiskKind {
			continue
		}

		if status.Status == v1alpha2.VMOPResourceStatusInProgress {
			return &reconcile.Result{}, nil
		}
	}

	if !object.IsTerminating(vmSnapshot) {
		err := s.client.Delete(ctx, vmSnapshot)
		if err != nil {
			return &reconcile.Result{}, fmt.Errorf("failed to delete the `VirtualMachineSnapshot`: %w", err)
		}
	}

	return &reconcile.Result{}, nil
}
