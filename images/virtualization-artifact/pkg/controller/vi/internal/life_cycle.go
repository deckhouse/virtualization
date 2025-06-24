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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type LifeCycleHandler struct {
	client      client.Client
	sources     Sources
	diskService DiskService
	recorder    eventrecord.EventRecorderLogger
}

func NewLifeCycleHandler(recorder eventrecord.EventRecorderLogger, sources Sources, client client.Client, diskService DiskService) *LifeCycleHandler {
	return &LifeCycleHandler{
		recorder:    recorder,
		client:      client,
		sources:     sources,
		diskService: diskService,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	readyCondition, ok := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)

	if !ok {
		cb := conditions.NewConditionBuilder(vicondition.ReadyType).
			Generation(vi.Generation).
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown)

		conditions.SetCondition(cb, &vi.Status.Conditions)
	}

	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)

	if vi.DeletionTimestamp != nil {
		// It is necessary to update this condition in order to use this image as a datasource.
		if readyCondition.Status == metav1.ConditionTrue {
			if vi.Spec.Storage == virtv2.StorageContainerRegistry {
				cb.
					Status(metav1.ConditionTrue).
					Reason(vicondition.Ready).
					Message("")
			} else {
				supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)
				pvc, err := h.diskService.GetPersistentVolumeClaim(ctx, supgen)
				if err != nil {
					return reconcile.Result{}, err
				}

				source.SetPhaseConditionForFinishedImage(pvc, cb, &vi.Status.Phase, supgen)
			}
		} else {
			cb.
				Status(readyCondition.Status).
				Reason(conditions.ReasonUnknown).
				Message("")
		}

		conditions.SetCondition(cb, &vi.Status.Conditions)
		vi.Status.Phase = virtv2.ImageTerminating
		return reconcile.Result{}, nil
	}

	if vi.Status.Phase == "" {
		vi.Status.Phase = virtv2.ImagePending
	}

	if readyCondition.Status != metav1.ConditionTrue && h.sources.Changed(ctx, vi) {
		h.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			virtv2.ReasonVISpecHasBeenChanged,
			"Spec changes are detected: import process is restarted by controller",
		)

		// Reset status and start import again.
		vi.Status = virtv2.VirtualImageStatus{
			Phase: virtv2.ImagePending,
		}

		_, err := h.sources.CleanUp(ctx, vi)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true}, nil
	}

	// TODO: Reconciliation in source handlers for ready images should not be blocked by a missing datasource.
	datasourceReadyCondition, _ := conditions.GetCondition(vicondition.DatasourceReadyType, vi.Status.Conditions)
	if datasourceReadyCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(datasourceReadyCondition, vi) {
		cb.
			Status(metav1.ConditionFalse).
			Reason(vicondition.DatasourceNotReady).
			Message("Datasource is not ready.")
		conditions.SetCondition(cb, &vi.Status.Conditions)

		return reconcile.Result{}, nil
	}

	if !source.IsImageProvisioningFinished(readyCondition) && (vi.Spec.Storage == virtv2.StorageKubernetes || vi.Spec.Storage == virtv2.StoragePersistentVolumeClaim) {
		storageClassReady, _ := conditions.GetCondition(vicondition.StorageClassReadyType, vi.Status.Conditions)
		if storageClassReady.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(storageClassReady, vi) {
			cb.
				Status(metav1.ConditionFalse).
				Reason(vicondition.StorageClassNotReady).
				Message("Storage class in not ready")
			conditions.SetCondition(cb, &vi.Status.Conditions)

			return reconcile.Result{}, nil
		}

		if vi.Status.StorageClassName == "" {
			return reconcile.Result{}, fmt.Errorf("empty storage class in status")
		}
	}

	ds, exists := h.sources.For(vi.Spec.DataSource.Type)
	if !exists {
		return reconcile.Result{}, fmt.Errorf("data source runner not found for type: %s", vi.Spec.DataSource.Type)
	}

	switch vi.Spec.Storage {
	case virtv2.StorageKubernetes, virtv2.StoragePersistentVolumeClaim:
		return ds.StoreToPVC(ctx, vi)
	case virtv2.StorageContainerRegistry:
		return ds.StoreToDVCR(ctx, vi)
	default:
		return reconcile.Result{}, fmt.Errorf("unknown spec storage: %s", vi.Spec.Storage)
	}
}

func (h LifeCycleHandler) Name() string {
	return "LifeCycleHandler"
}
