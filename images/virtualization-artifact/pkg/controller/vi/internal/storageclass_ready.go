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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type StorageclassReadyHandler struct {
	service *service.DiskService
}

func (h StorageclassReadyHandler) Name() string {
	return "StorageclassReadyHandler"
}

func NewStorageClassReadyHandler(client service.Client) *StorageclassReadyHandler {
	return &StorageclassReadyHandler{
		service: service.NewDiskService(client, nil, nil),
	}
}

func (h StorageclassReadyHandler) Handle(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	condition, ok := service.GetCondition(vicondition.StorageclassReadyType, vi.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vicondition.StorageclassReadyType,
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
	case v1alpha2.StorageContainerRegistry:
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.DVCRTypeUsed
		condition.Message = "Used dvcr storage"
	case v1alpha2.StorageKubernetes:
		sc, err := h.service.GetStorageClass(ctx, vi.Spec.PersistentVolumeClaim.StorageClass)
		if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) && !errors.Is(err, service.ErrStorageClassNotFound) {
			return reconcile.Result{}, err
		}

		if sc != nil {
			condition.Status = metav1.ConditionTrue
			condition.Reason = vicondition.StorageclassReady
			condition.Message = "Storageclass ready"
		} else {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vicondition.StorageclassNotReady
			condition.Message = fmt.Sprintf("Storageclass %q not ready", *vi.Spec.PersistentVolumeClaim.StorageClass)
		}
	}

	return reconcile.Result{}, nil
}
