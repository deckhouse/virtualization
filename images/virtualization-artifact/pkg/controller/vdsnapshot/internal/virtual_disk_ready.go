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

package internal

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
)

type VirtualDiskReadyHandler struct {
	snapshotter VirtualDiskReadySnapshotter
}

func NewVirtualDiskReadyHandler(snapshotter VirtualDiskReadySnapshotter) *VirtualDiskReadyHandler {
	return &VirtualDiskReadyHandler{
		snapshotter: snapshotter,
	}
}

func (h VirtualDiskReadyHandler) Handle(ctx context.Context, vdSnapshot *virtv2.VirtualDiskSnapshot) (reconcile.Result, error) {
	condition, ok := service.GetCondition(vdscondition.VirtualDiskReadyType, vdSnapshot.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vdscondition.VirtualDiskReadyType,
			Status: metav1.ConditionUnknown,
			Reason: conditions.ReasonUnknown.String(),
		}
	}

	defer func() { service.SetCondition(condition, &vdSnapshot.Status.Conditions) }()

	if vdSnapshot.DeletionTimestamp != nil {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = conditions.ReasonUnknown.String()
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	if vdSnapshot.Status.Phase == virtv2.VirtualDiskSnapshotPhaseReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdscondition.VirtualDiskReady
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	vd, err := h.snapshotter.GetVirtualDisk(ctx, vdSnapshot.Spec.VirtualDiskName, vdSnapshot.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vd == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdscondition.VirtualDiskNotReadyForSnapshotting
		condition.Message = fmt.Sprintf("The virtual disk %q not found.", vdSnapshot.Spec.VirtualDiskName)
		return reconcile.Result{}, nil
	}

	if vd.GetDeletionTimestamp() != nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdscondition.VirtualDiskNotReadyForSnapshotting
		condition.Message = fmt.Sprintf("The virtual disk %q is in process of deletion.", vd.Name)
		return reconcile.Result{}, nil
	}

	switch vd.Status.Phase {
	case virtv2.DiskReady:
		snapshotting, _ := conditions.GetCondition(vdcondition.SnapshottingType, vd.Status.Conditions)
		if snapshotting.Status != metav1.ConditionTrue {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdscondition.VirtualDiskNotReadyForSnapshotting
			condition.Message = snapshotting.Message
			return reconcile.Result{}, nil
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = vdscondition.VirtualDiskReady
		condition.Message = ""
		return reconcile.Result{}, nil
	default:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdscondition.VirtualDiskNotReadyForSnapshotting
		condition.Message = fmt.Sprintf("The virtual disk %q is in the %q phase: waiting for it to reach the Ready phase.", vd.Name, vd.Status.Phase)
		return reconcile.Result{}, nil
	}
}
