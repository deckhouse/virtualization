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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
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
	cb := conditions.NewConditionBuilder(vdcondition.StorageClassReadyType).Generation(vd.Generation)

	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	if vd.DeletionTimestamp != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")
		return reconcile.Result{}, nil
	}

	supgen := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)
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
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.StorageClassReady).
			Message("")
	case hasNoStorageClassInSpec:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotFound).
			Message("The default storage class was not found in cluster. Please specify the storage class name in the virtual disk specification.")
	default:
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotFound).
			Message(fmt.Sprintf("Storage class %q not found", *vd.Spec.PersistentVolumeClaim.StorageClass))
	}

	return reconcile.Result{}, nil
}
