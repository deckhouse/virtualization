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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type StorageClassReadyHandler struct {
	service  DiskService
	recorder eventrecord.EventRecorderLogger
}

func (h StorageClassReadyHandler) Name() string {
	return "StorageClassReadyHandler"
}

func NewStorageClassReadyHandler(recorder eventrecord.EventRecorderLogger, diskService DiskService) *StorageClassReadyHandler {
	return &StorageClassReadyHandler{
		service:  diskService,
		recorder: recorder,
	}
}

func (h StorageClassReadyHandler) Handle(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vicondition.StorageClassReadyType).Generation(vi.Generation)

	defer func() { conditions.SetCondition(cb, &vi.Status.Conditions) }()

	if vi.DeletionTimestamp != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message("")
	}

	if vi.Spec.Storage == virtv2.StorageContainerRegistry {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(vicondition.DVCRTypeUsed).
			Message("Used dvcr storage")
		return reconcile.Result{}, nil
	}

	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pvc, err := h.service.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	hasNoStorageClassInSpec := vi.Spec.PersistentVolumeClaim.StorageClass == nil || *vi.Spec.PersistentVolumeClaim.StorageClass == ""
	hasStorageClassInStatus := vi.Status.StorageClassName != ""
	var storageClassName *string

	sc, err := h.service.GetStorageClass(ctx, storageClassName)
	if err != nil && !errors.Is(err, service.ErrDefaultStorageClassNotFound) && !errors.Is(err, service.ErrStorageClassNotFound) {
		return reconcile.Result{}, err
	}

	switch {
	case pvc != nil && pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "":
		vi.Status.StorageClassName = *pvc.Spec.StorageClassName
	case vi.Spec.PersistentVolumeClaim.StorageClass != nil && *vi.Spec.PersistentVolumeClaim.StorageClass != "":
		vi.Status.StorageClassName = *vi.Spec.PersistentVolumeClaim.StorageClass
	case !hasStorageClassInStatus && sc != nil:
		vi.Status.StorageClassName = sc.Name
	}

	switch {
	case sc != nil:
		cb.
			Status(metav1.ConditionTrue).
			Reason(vicondition.StorageClassReady).
			Message("")
	case hasNoStorageClassInSpec:
		h.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			virtv2.ReasonVDStorageClassNotFound,
			"The default storage class was not found in cluster. Please specify the storage class name in the virtual disk specification.",
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotFound).
			Message("The default storage class was not found in cluster. Please specify the storage class name in the virtual image or virtualization module config specification.")
	default:
		h.recorder.Eventf(
			vi,
			corev1.EventTypeNormal,
			virtv2.ReasonVDStorageClassNotFound,
			"Storage class %q not found", *vi.Spec.PersistentVolumeClaim.StorageClass,
		)
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.StorageClassNotFound).
			Message(fmt.Sprintf("Storage class %q not found", *vi.Spec.PersistentVolumeClaim.StorageClass))
	}

	return reconcile.Result{}, nil
}
