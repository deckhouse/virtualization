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

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type StorageClassReadyHandler struct {
	service DiskService
	sources *source.Sources
}

func NewStorageClassReadyHandler(diskService DiskService, sources *source.Sources) *StorageClassReadyHandler {
	return &StorageClassReadyHandler{
		service: diskService,
		sources: sources,
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

	isDefaultStorageClass := vd.Spec.PersistentVolumeClaim.StorageClass == nil || *vd.Spec.PersistentVolumeClaim.StorageClass == ""
	specClassChanged := false
	if isDefaultStorageClass {
		if vd.Status.StorageClassName == "" {
			sc, _ := h.service.GetStorageClass(ctx, nil)
			if sc != nil {
				vd.Status.StorageClassName = sc.Name
			}
		}
	} else {
		if vd.Status.StorageClassName == "" {
			vd.Status.StorageClassName = *vd.Spec.PersistentVolumeClaim.StorageClass
		}
		if vd.Status.StorageClassName != *vd.Spec.PersistentVolumeClaim.StorageClass {
			specClassChanged = true
		}
	}

	var sc *storagev1.StorageClass
	if vd.Status.StorageClassName != "" {
		var err error
		sc, err = h.service.GetStorageClass(ctx, &vd.Status.StorageClassName)
		if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) && !errors.Is(err, service.ErrStorageClassNotFound) {
			return reconcile.Result{}, err
		}
	}

	switch {
	case sc != nil:
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.StorageClassReady
		condition.Message = ""
	case isDefaultStorageClass:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.StorageClassNameNotProvided
		condition.Message = "Storage class not provided and default storage class not found."
	default:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.StorageClassNotFound
		condition.Message = fmt.Sprintf("Storage class %q not found", *vd.Spec.PersistentVolumeClaim.StorageClass)
	}

	if condition.Status != metav1.ConditionTrue || specClassChanged {
		vd.Status.Phase = virtv2.DiskPending
		_, err := h.sources.CleanUp(ctx, vd)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to clean up to restart import process: %w", err)
		}
	}

	if condition.Status != metav1.ConditionTrue && isDefaultStorageClass && vd.Status.StorageClassName != "" {
		vd.Status.StorageClassName = ""
	}

	return reconcile.Result{}, nil
}
