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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type SnapshottingHandler struct {
	diskService *service.DiskService
}

func NewSnapshottingHandler(diskService *service.DiskService) *SnapshottingHandler {
	return &SnapshottingHandler{
		diskService: diskService,
	}
}

func (h SnapshottingHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vdcondition.SnapshottingType).Generation(vd.Generation)

	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	if vd.DeletionTimestamp != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")
		return reconcile.Result{}, nil
	}

	readyCondition, ok := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !ok || readyCondition.Status != metav1.ConditionTrue {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")
		return reconcile.Result{}, nil
	}

	vdSnapshots, err := h.diskService.ListVirtualDiskSnapshots(ctx, vd.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, vdSnapshot := range vdSnapshots {
		if vdSnapshot.Spec.VirtualDiskName != vd.Name {
			continue
		}

		if vdSnapshot.Status.Phase == virtv2.VirtualDiskSnapshotPhaseReady || vdSnapshot.Status.Phase == virtv2.VirtualDiskSnapshotPhaseTerminating {
			continue
		}

		resizing, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		if resizing.Status == metav1.ConditionTrue && vd.Generation == resizing.ObservedGeneration {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.SnapshottingNotAvailable).
				Message("The virtual disk cannot be selected for snapshotting as it is currently resizing.")
			return reconcile.Result{}, nil
		}

		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Snapshotting).
			Message("The virtual disk is selected for taking a snapshot.")
		return reconcile.Result{}, nil
	}

	cb.
		Status(metav1.ConditionUnknown).
		Reason(conditions.ReasonUnknown).
		Message("")
	return reconcile.Result{}, nil
}
