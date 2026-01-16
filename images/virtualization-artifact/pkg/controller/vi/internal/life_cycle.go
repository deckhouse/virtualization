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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type LifeCycleHandler struct {
	client   client.Client
	sources  Sources
	recorder eventrecord.EventRecorderLogger
}

func NewLifeCycleHandler(recorder eventrecord.EventRecorderLogger, sources Sources, client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		recorder: recorder,
		client:   client,
		sources:  sources,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	readyCondition, ok := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)

	if !ok {
		cb := conditions.NewConditionBuilder(vicondition.ReadyType).
			Generation(vi.Generation).
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown)

		conditions.SetCondition(cb, &vi.Status.Conditions)
	}

	if vi.DeletionTimestamp != nil {
		vi.Status.Phase = v1alpha2.ImageTerminating
		return reconcile.Result{}, nil
	}

	if vi.Status.Phase == v1alpha2.ImageLost || vi.Status.Phase == v1alpha2.ImagePVCLost {
		return reconcile.Result{}, nil
	}

	if vi.Status.Phase == "" {
		vi.Status.Phase = v1alpha2.ImagePending
	}

	if readyCondition.Status != metav1.ConditionTrue && h.sources.Changed(ctx, vi) {
		h.recorder.Event(
			vi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVISpecHasBeenChanged,
			"Spec changes are detected: import process is restarted by controller",
		)

		// Reset status and start import again.
		vi.Status = v1alpha2.VirtualImageStatus{
			Phase: v1alpha2.ImagePending,
		}

		_, err := h.sources.CleanUp(ctx, vi)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true}, nil
	}

	cb := conditions.NewConditionBuilder(vicondition.ReadyType).Generation(vi.Generation)

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

	if !source.IsImageProvisioningFinished(readyCondition) && (vi.Spec.Storage == v1alpha2.StorageKubernetes || vi.Spec.Storage == v1alpha2.StoragePersistentVolumeClaim) {
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
	case v1alpha2.StorageKubernetes, v1alpha2.StoragePersistentVolumeClaim:
		return ds.StoreToPVC(ctx, vi)
	case v1alpha2.StorageContainerRegistry:
		return ds.StoreToDVCR(ctx, vi)
	default:
		return reconcile.Result{}, fmt.Errorf("unknown spec storage: %s", vi.Spec.Storage)
	}
}

func (h LifeCycleHandler) Name() string {
	return "LifeCycleHandler"
}
