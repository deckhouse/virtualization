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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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

func (h SnapshottingHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vdcondition.SnapshottingType).Generation(vd.Generation)

	defer func() {
		if cb.Condition().Status != metav1.ConditionUnknown {
			conditions.SetCondition(cb, &vd.Status.Conditions)
		} else {
			conditions.RemoveCondition(cb.GetType(), &vd.Status.Conditions)
		}
	}()

	if vd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !conditions.IsLastUpdated(readyCondition, vd) || readyCondition.Status != metav1.ConditionTrue {
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

		if vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseReady || vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseTerminating || vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseFailed {
			continue
		}

		resizing, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
		if resizing.Status == metav1.ConditionTrue && conditions.IsLastUpdated(resizing, vd) {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.SnapshottingNotAvailable).
				Message("The virtual disk cannot be selected for snapshotting as it is currently resizing.")
			return reconcile.Result{}, nil
		}

		migrating, _ := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
		if migrating.Status == metav1.ConditionTrue && conditions.IsLastUpdated(migrating, vd) {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vdcondition.SnapshottingNotAvailable).
				Message("The virtual disk cannot be selected for snapshotting as it is currently being migrated.")
			return reconcile.Result{}, nil
		}

		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.Snapshotting).
			Message("The virtual disk is selected for taking a snapshot.")
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}
