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
}

func (h StorageClassReadyHandler) Name() string {
	return "StorageClassReadyHandler"
}

func NewStorageClassReadyHandler(diskService DiskService) *StorageClassReadyHandler {
	return &StorageClassReadyHandler{
		service: diskService,
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

	switch vi.Spec.Storage {
	case virtv2.StorageContainerRegistry:
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.DVCRTypeUsed
		condition.Message = "Used dvcr storage"
	case virtv2.StorageKubernetes:
		sc, err := h.service.GetStorageClass(ctx, vi.Spec.PersistentVolumeClaim.StorageClass)
		if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) && !errors.Is(err, service.ErrStorageClassNotFound) {
			return reconcile.Result{}, err
		}

		switch {
		case sc != nil:
			condition.Status = metav1.ConditionTrue
			condition.Reason = vicondition.StorageClassReady
			condition.Message = ""
		case vi.Spec.PersistentVolumeClaim.StorageClass == nil || *vi.Spec.PersistentVolumeClaim.StorageClass == "":
			condition.Status = metav1.ConditionFalse
			condition.Reason = vicondition.StorageClassNameNotProvided
			condition.Message = "Storage class not provided and default storage class not found."
		default:
			condition.Status = metav1.ConditionFalse
			condition.Reason = vicondition.StorageClassNotFound
			condition.Message = fmt.Sprintf("StorageClass %q not ready", *vi.Spec.PersistentVolumeClaim.StorageClass)
		}
	}

	return reconcile.Result{}, nil
}
