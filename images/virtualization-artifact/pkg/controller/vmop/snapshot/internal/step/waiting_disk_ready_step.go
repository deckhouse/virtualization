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
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type WaitingDisksReadyStep struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func NewWaitingDisksStep(
	client client.Client,
	recorder eventrecord.EventRecorderLogger,
) *WaitingDisksReadyStep {
	return &WaitingDisksReadyStep{
		client:   client,
		recorder: recorder,
	}
}

func (s WaitingDisksReadyStep) Take(ctx context.Context, vmop *v1alpha2.VirtualMachineOperation) (*reconcile.Result, error) {
	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeRestore:
		if vmop.Spec.Restore.Mode == v1alpha2.SnapshotOperationModeDryRun {
			return nil, nil
		}
	case v1alpha2.VMOPTypeClone:
		if vmop.Spec.Clone.Mode == v1alpha2.SnapshotOperationModeDryRun {
			return nil, nil
		}
	}

	cb := conditions.NewConditionBuilder(vmopcondition.TypeCompleted).Status(metav1.ConditionFalse)
	switch vmop.Spec.Type {
	case v1alpha2.VMOPTypeClone:
		cb.Reason(vmopcondition.ReasonCloneInProgress)
	case v1alpha2.VMOPTypeRestore:
		cb.Reason(vmopcondition.ReasonRestoreInProgress)
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
			// Skip disks that don't belong to this vmop
			continue
		}

		if vd.Status.Phase == v1alpha2.DiskFailed {
			return &reconcile.Result{}, fmt.Errorf("virtual disk %q is in failed phase", vdKey.Name)
		}

		switch vd.Status.Phase {
		case v1alpha2.DiskFailed:
			return &reconcile.Result{}, fmt.Errorf("virtual disk %q is in failed phase", vdKey.Name)
		case v1alpha2.DiskReady:
			// Disk is Ready, check the next one.
			continue
		case v1alpha2.DiskWaitForFirstConsumer:
			if vmop.Spec.Type == v1alpha2.VMOPTypeClone {
				cb.Message(fmt.Sprintf("%s operation is completed. Waiting for resource readiness. Waiting for cleanup.", vmop.Spec.Type))
				conditions.SetCondition(cb, &vmop.Status.Conditions)
				return &reconcile.Result{}, nil // Should wait for disk ready.
			}
			continue
		default:
			cb.Message(fmt.Sprintf("%s operation is completed. Waiting for resource readiness. Waiting for cleanup.", vmop.Spec.Type))
			conditions.SetCondition(cb, &vmop.Status.Conditions)
			return &reconcile.Result{}, nil
		}
	}

	cb.Message(fmt.Sprintf("%s operation is completed. Resources are ready. Waiting for cleanup.", vmop.Spec.Type))
	conditions.SetCondition(cb, &vmop.Status.Conditions)

	return nil, nil
}
