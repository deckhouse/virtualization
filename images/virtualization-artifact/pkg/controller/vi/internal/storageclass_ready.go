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

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type StorageClassReadyHandler struct {
	service DiskService
	sources Sources
}

func (h StorageClassReadyHandler) Name() string {
	return "StorageClassReadyHandler"
}

func NewStorageClassReadyHandler(diskService DiskService, sources Sources) *StorageClassReadyHandler {
	return &StorageClassReadyHandler{
		service: diskService,
		sources: sources,
	}
}

func (h StorageClassReadyHandler) Handle(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	condition, ok := service.GetCondition(vicondition.StorageClassReadyType, vi.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vicondition.StorageClassReadyType,
			Status: metav1.ConditionUnknown,
		}
	}

	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	if vi.DeletionTimestamp != nil {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	if vi.Spec.Storage == virtv2.StorageContainerRegistry {
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.DVCRTypeUsed
		condition.Message = "Used dvcr storage"
		return reconcile.Result{}, nil
	}

	isDefaultStorageClass := vi.Spec.PersistentVolumeClaim.StorageClass == nil || *vi.Spec.PersistentVolumeClaim.StorageClass == ""
	hasStorageClassInStatus := vi.Status.StorageClassName != ""
	var checkedSCName *string
	if hasStorageClassInStatus && isDefaultStorageClass {
		checkedSCName = &vi.Status.StorageClassName
	} else {
		checkedSCName = vi.Spec.PersistentVolumeClaim.StorageClass
	}

	sc, err := h.service.GetStorageClass(ctx, checkedSCName)
	if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) && !errors.Is(err, service.ErrStorageClassNotFound) {
		return reconcile.Result{}, err
	}

	if !hasStorageClassInStatus && sc != nil {
		vi.Status.StorageClassName = sc.Name
	}

	switch {
	case sc != nil:
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.StorageClassReady
		condition.Message = ""
	case isDefaultStorageClass:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.StorageClassNotFound
		condition.Message = "The default storage class was not found in cluster. Please specify the storage class name in the virtual disk specification."
	default:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.StorageClassNotFound
		condition.Message = fmt.Sprintf("Storage class %q not found", *vi.Spec.PersistentVolumeClaim.StorageClass)
	}

	return reconcile.Result{}, nil
}
