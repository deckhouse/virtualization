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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type StorageClassReadyHandler struct {
	service DiskService
}

func NewStorageClassReadyHandler(diskService DiskService) *StorageClassReadyHandler {
	return &StorageClassReadyHandler{
		service: diskService,
	}
}

func (h StorageClassReadyHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	condition, ok := service.GetCondition(vdcondition.StorageClassReadyType, vd.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vdcondition.StorageClassReadyType,
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

	supgen := supplements.NewGenerator(cc.VDShortName, vd.Name, vd.Namespace, vd.UID)
	pvc, err := h.service.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	hasNoStorageClassInSpec := vd.Spec.PersistentVolumeClaim.StorageClass == nil || *vd.Spec.PersistentVolumeClaim.StorageClass == ""
	hasStorageClassInStatus := vd.Status.StorageClassName != ""
	var storageClassName *string
	switch {
	case pvc != nil && pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "":
		storageClassName = pvc.Spec.StorageClassName
	case hasStorageClassInStatus && hasNoStorageClassInSpec:
		storageClassName = &vd.Status.StorageClassName
	default:
		storageClassName = vd.Spec.PersistentVolumeClaim.StorageClass
	}

	sc, err := h.service.GetStorageClass(ctx, storageClassName)
	if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) && !errors.Is(err, service.ErrStorageClassNotFound) {
		return reconcile.Result{}, err
	}

	switch {
	case pvc != nil && pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "":
		vd.Status.StorageClassName = *pvc.Spec.StorageClassName
	case vd.Spec.PersistentVolumeClaim.StorageClass != nil:
		vd.Status.StorageClassName = *vd.Spec.PersistentVolumeClaim.StorageClass
	case !hasStorageClassInStatus && sc != nil:
		vd.Status.StorageClassName = sc.Name
	}

	switch {
	case sc != nil:
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.StorageClassReady
		condition.Message = ""
	case hasNoStorageClassInSpec:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.StorageClassNotFound
		condition.Message = "The default storage class was not found in cluster. Please specify the storage class name in the virtual disk specification."
	default:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.StorageClassNotFound
		condition.Message = fmt.Sprintf("Storage class %q not found", *vd.Spec.PersistentVolumeClaim.StorageClass)
	}

	return reconcile.Result{}, nil
}
