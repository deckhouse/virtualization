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

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ResizingHandler struct {
	diskService DiskService
}

func NewResizingHandler(diskService DiskService) *ResizingHandler {
	return &ResizingHandler{
		diskService: diskService,
	}
}

func (h ResizingHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("resizing"))

	condition, ok := service.GetCondition(vdcondition.ResizedType, vd.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vdcondition.ResizedType,
			Status: metav1.ConditionUnknown,
		}
	}

	defer func() { service.SetCondition(condition, &vd.Status.Conditions) }()

	if vd.DeletionTimestamp != nil {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	readyCondition, ok := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !ok || readyCondition.Status != metav1.ConditionTrue {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
	pvc, err := h.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	if pvc == nil {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = "Underlying PersistentVolumeClaim not found: resizing is not possible."
		return reconcile.Result{}, nil
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = "Underlying PersistentVolumeClaim not bound: resizing is not possible."
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
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.InProgress
		condition.Message = pvcResizing.Message
		return reconcile.Result{}, nil
	}

	// Expected disk size is GREATER THAN expected pvc size: resize needed, resizing to a larger size.
	if vdSpecSize != nil && vdSpecSize.Cmp(pvcSpecSize) == 1 {
		snapshotting, _ := service.GetCondition(vdcondition.SnapshottingType, vd.Status.Conditions)
		if snapshotting.Status == metav1.ConditionTrue {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ResizingNotAvailable
			condition.Message = "The virtual disk cannot be selected for resizing as it is currently snapshotting."
			return reconcile.Result{}, nil
		}

		err = h.diskService.Resize(ctx, pvc, *vdSpecSize)
		if err != nil {
			if k8serrors.IsForbidden(err) {
				condition.Status = metav1.ConditionFalse
				condition.Reason = vdcondition.ResizingNotAvailable
				condition.Message = fmt.Sprintf("Disk resizing is not allowed: %s.", err.Error())
				return reconcile.Result{}, nil
			}
			return reconcile.Result{}, err
		}

		log.Info("The virtual disk resizing has started")

		vd.Status.Phase = virtv2.DiskResizing
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.InProgress
		condition.Message = "The virtual disk resizing has started."
		return reconcile.Result{}, nil
	}

	// Expected disk size is NOT GREATER THAN expected pvc size: no resize needed since downsizing is not possible, and resizing to the same value makes no sense.
	switch condition.Reason {
	case vdcondition.InProgress, vdcondition.Resized:
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.Resized
		condition.Message = ""
	default:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ResizingNotRequested
		condition.Message = ""
	}

	return reconcile.Result{}, nil
}
