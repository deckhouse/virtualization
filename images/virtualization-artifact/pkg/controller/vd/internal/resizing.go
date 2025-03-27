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
	"log/slog"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ResizingHandler struct {
	diskService DiskService
	recorder    eventrecord.EventRecorderLogger
}

func NewResizingHandler(recorder eventrecord.EventRecorderLogger, diskService DiskService) *ResizingHandler {
	return &ResizingHandler{
		diskService: diskService,
		recorder:    recorder,
	}
}

func (h ResizingHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("resizing"))

	resizingCondition, _ := conditions.GetCondition(vdcondition.ResizingType, vd.Status.Conditions)
	cb := conditions.NewConditionBuilder(vdcondition.ResizingType).Generation(vd.Generation)

	if vd.DeletionTimestamp != nil {
		conditions.RemoveCondition(cb.GetType(), &vd.Status.Conditions)
		return reconcile.Result{}, nil
	}

	readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !conditions.IsLastUpdated(readyCondition, vd) || readyCondition.Status != metav1.ConditionTrue {
		conditions.RemoveCondition(cb.GetType(), &vd.Status.Conditions)
		return reconcile.Result{}, nil
	}

	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)
	pvc, err := h.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		conditions.RemoveCondition(cb.GetType(), &vd.Status.Conditions)
		return reconcile.Result{}, err
	}

	if pvc == nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("Underlying PersistentVolumeClaim not found: resizing is not possible.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return reconcile.Result{}, nil
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("Underlying PersistentVolumeClaim not bound: resizing is not possible.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return reconcile.Result{}, nil
	}

	vdSpecSize := vd.Spec.PersistentVolumeClaim.Size
	pvcSpecSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	pvcStatusSize := pvc.Status.Capacity[corev1.ResourceStorage]

	log.With("vdSpecSize", vdSpecSize, "pvcSpecSize", pvcSpecSize.String(), "pvcStatusSize", pvcStatusSize)

	// Synchronize VirtualDisk size with PVC size.
	vd.Status.Capacity = pvcStatusSize.String()

	pvcResizing := service.GetPersistentVolumeClaimCondition(corev1.PersistentVolumeClaimResizing, pvc.Status.Conditions)
	if pvcResizing != nil && pvcResizing.Status == corev1.ConditionTrue {
		log.Info("Resizing is in progress", "msg", pvcResizing.Message)

		vd.Status.Phase = virtv2.DiskResizing
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.InProgress).
			Message(pvcResizing.Message)
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return reconcile.Result{}, nil
	}

	if isResizeNeeded(vdSpecSize, &pvcSpecSize) {
		// Expected disk size is GREATER THAN expected pvc size: resize needed, resizing to a larger size.
		return h.ResizeNeeded(ctx, vd, pvc, cb, log)
	} else {
		// Expected disk size is NOT GREATER THAN expected pvc size: no resize needed since downsizing is not possible, and resizing to the same value makes no sense.
		return h.ResizeNotNeeded(vd, resizingCondition, cb)
	}
}

func (h ResizingHandler) ResizeNeeded(
	ctx context.Context,
	vd *virtv2.VirtualDisk,
	pvc *corev1.PersistentVolumeClaim,
	cb *conditions.ConditionBuilder,
	log *slog.Logger,
) (reconcile.Result, error) {
	snapshotting, _ := conditions.GetCondition(vdcondition.SnapshottingType, vd.Status.Conditions)

	if snapshotting.Status == metav1.ConditionTrue && conditions.IsLastUpdated(snapshotting, vd) {
		h.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			virtv2.ReasonVDResizingNotAvailable,
			"The virtual disk cannot be selected for resizing as it is currently snapshotting.",
		)

		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ResizingNotAvailable).
			Message("The virtual disk cannot be selected for resizing as it is currently snapshotting.")

		conditions.SetCondition(cb, &vd.Status.Conditions)
		return reconcile.Result{}, nil
	}

	storageClassReadyCondition, _ := conditions.GetCondition(vdcondition.StorageClassReadyType, vd.Status.Conditions)
	if !conditions.IsLastUpdated(storageClassReadyCondition, vd) {
		storageClassReadyCondition.Status = metav1.ConditionUnknown
	}

	switch storageClassReadyCondition.Status {
	case metav1.ConditionTrue:
		if vd.Spec.PersistentVolumeClaim.Size == nil {
			conditions.RemoveCondition(cb.GetType(), &vd.Status.Conditions)
			return reconcile.Result{}, errors.New("PersistentVolumeClaim does not have a size")
		}

		err := h.diskService.Resize(ctx, pvc, *vd.Spec.PersistentVolumeClaim.Size)
		if err != nil {
			if k8serrors.IsForbidden(err) {
				cb.
					Status(metav1.ConditionFalse).
					Reason(vdcondition.ResizingNotAvailable).
					Message(fmt.Sprintf("Disk resizing is not allowed: %s.", err.Error()))
				conditions.SetCondition(cb, &vd.Status.Conditions)
			} else {
				conditions.RemoveCondition(cb.GetType(), &vd.Status.Conditions)
			}
			return reconcile.Result{}, err
		}

		h.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			virtv2.ReasonVDResizingStarted,
			"The virtual disk resizing has started",
		)

		log.Info("The virtual disk resizing has started")

		vd.Status.Phase = virtv2.DiskResizing
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.InProgress).
			Message("The virtual disk resizing has started.")
		conditions.SetCondition(cb, &vd.Status.Conditions)
	case metav1.ConditionFalse, metav1.ConditionUnknown:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ResizingNotAvailable).
			Message("Disk resizing is not allowed: Storage class is not ready")
		conditions.SetCondition(cb, &vd.Status.Conditions)
	}

	return reconcile.Result{}, nil
}

func (h ResizingHandler) ResizeNotNeeded(
	vd *virtv2.VirtualDisk,
	resizingCondition metav1.Condition,
	cb *conditions.ConditionBuilder,
) (reconcile.Result, error) {
	if resizingCondition.Status == metav1.ConditionTrue {
		h.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			virtv2.ReasonVDResizingCompleted,
			"The virtual disk resizing has completed",
		)
	}

	conditions.RemoveCondition(cb.GetType(), &vd.Status.Conditions)
	return reconcile.Result{}, nil
}

func isResizeNeeded(vdSpecSize, pvcSpecSize *resource.Quantity) bool {
	return vdSpecSize != nil && pvcSpecSize != nil && vdSpecSize.Cmp(*pvcSpecSize) == common.CmpGreater
}
