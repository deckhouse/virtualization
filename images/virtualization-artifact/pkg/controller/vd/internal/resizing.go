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
	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ResizingHandler struct {
	diskService *service.DiskService
}

func NewResizingHandler(diskService *service.DiskService) *ResizingHandler {
	return &ResizingHandler{
		diskService: diskService,
	}
}

func (h ResizingHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
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

	newSize := vd.Spec.PersistentVolumeClaim.Size
	if newSize == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ResizedReason_NotRequested
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	readyCondition, ok := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !ok || readyCondition.Status != metav1.ConditionTrue {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ResizedReason_NotRequested
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
	pvc, err := h.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	if pvc == nil {
		return reconcile.Result{}, errors.New("pvc not found for ready virtual disk")
	}

	if newSize.Equal(pvc.Status.Capacity[corev1.ResourceStorage]) {
		if condition.Reason == vdcondition.ResizedReason_InProgress {
			condition.Status = metav1.ConditionTrue
			condition.Reason = vdcondition.ResizedReason_Resized
			condition.Message = ""
			return reconcile.Result{}, nil
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ResizedReason_NotRequested
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	err = h.diskService.Resize(ctx, pvc, *newSize, supgen)
	switch {
	case err == nil:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ResizedReason_InProgress
		condition.Message = "The virtual disk is in the process of resizing."
		return reconcile.Result{}, nil
	case errors.Is(err, service.ErrTooSmallDiskSize):
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ResizedReason_TooSmallDiskSize
		condition.Message = "The new size of the virtual disk must not be smaller than the current size."
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, err
	}
}
