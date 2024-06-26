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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
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

	vdSpecSize := vd.Spec.PersistentVolumeClaim.Size
	if vdSpecSize == nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.NotRequested
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	readyCondition, ok := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !ok || readyCondition.Status != metav1.ConditionTrue {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.NotRequested
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

	pvcSpecSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	switch vdSpecSize.Cmp(pvcSpecSize) {
	// Expected disk size is LESS THAN expected pvc size: no resize needed as resizing to a smaller size is not possible.
	case -1:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.NotRequested
		condition.Message = ""
		return reconcile.Result{}, nil
	// Expected disk size is GREATER THAN expected pvc size: resize needed, resizing to a larger size.
	case 1:
		err = h.diskService.Resize(ctx, pvc, *vdSpecSize)
		if err != nil {
			return reconcile.Result{}, err
		}

		vd.Status.Phase = virtv2.DiskResizing

		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.InProgress
		condition.Message = "The virtual disk resizing has started."
		return reconcile.Result{}, nil
	// Expected disk size is EQUAL TO expected pvc size: cannot definitively say whether the resize has already happened or was not needed - perform additional checks.
	case 0:
	}

	var vdStatusSize resource.Quantity
	vdStatusSize, err = resource.ParseQuantity(vd.Status.Capacity)
	if err != nil {
		return reconcile.Result{}, err
	}

	pvcStatusSize := pvc.Status.Capacity[corev1.ResourceStorage]

	// Expected pvc size is GREATER THAN actual pvc size: resize has been requested and is in progress.
	if pvcSpecSize.Cmp(pvcStatusSize) == 1 {
		vd.Status.Phase = virtv2.DiskResizing

		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.InProgress
		condition.Message = "The virtual disk is in the process of resizing."
		return reconcile.Result{}, nil
	}

	// Virtual disk size DOES NOT MATCH pvc size: resize has completed, synchronize the virtual disk size.
	if !vdStatusSize.Equal(pvcStatusSize) {
		vd.Status.Capacity = pvcStatusSize.String()
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.Resized
		condition.Message = ""
		return reconcile.Result{Requeue: true}, nil
	}

	// Expected pvc size is NOT GREATER THAN actual PVC size AND virtual disk size MATCHES pvc size: resize was not requested.
	condition.Status = metav1.ConditionFalse
	condition.Reason = vdcondition.NotRequested
	condition.Message = ""
	return reconcile.Result{}, nil
}
