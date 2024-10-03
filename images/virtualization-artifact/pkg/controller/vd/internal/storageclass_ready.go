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
	"time"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type StorageclassReadyHandler struct {
	service *service.DiskService
}

func NewStorageclassReadyHandler(client client.Client) *StorageclassReadyHandler {
	return &StorageclassReadyHandler{
		service: service.NewDiskService(client, nil, nil),
	}
}

func (h StorageclassReadyHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	condition, ok := service.GetCondition(vdcondition.StorageclassReadyType, vd.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vdcondition.StorageclassReadyType,
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

	sc, err := h.service.GetStorageClass(ctx, vd.Spec.PersistentVolumeClaim.StorageClass)
	if err != nil && !errors.Is(err, errors.New("storage class not found")) {
		return reconcile.Result{}, err
	}

	if sc != nil {
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.StorageclassReady
		condition.Message = "Storage class ready."
	} else {
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.StorageclassNotReady
		condition.Message = fmt.Sprintf("Storage class %q not ready", *vd.Spec.PersistentVolumeClaim.StorageClass)
		return reconcile.Result{RequeueAfter: 2 * time.Second}, err
	}

	return reconcile.Result{}, nil
}
