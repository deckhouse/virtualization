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
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type LifeCycleHandler struct {
	client   client.Client
	sources  *source.Sources
	recorder eventrecord.EventRecorderLogger
}

func NewLifeCycleHandler(recorder eventrecord.EventRecorderLogger, sources *source.Sources, client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		client:   client,
		sources:  sources,
		recorder: recorder,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	readyCondition, ok := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	if !ok {
		cb := conditions.NewConditionBuilder(cvicondition.ReadyType).
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Generation(cvi.Generation)
		conditions.SetCondition(cb, &cvi.Status.Conditions)

		readyCondition = cb.Condition()
	}

	if cvi.DeletionTimestamp != nil {
		cvi.Status.Phase = v1alpha2.ImageTerminating
		return reconcile.Result{}, nil
	}

	if cvi.Status.Phase == v1alpha2.ImageLost {
		// Upload images cannot be recovered automatically: the data was uploaded once and cannot be re-fetched.
		if cvi.Spec.DataSource.Type == v1alpha2.DataSourceTypeUpload {
			return reconcile.Result{}, nil
		}

		h.recorder.Event(
			cvi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonCVIImageLostRecovering,
			"The image is lost in DVCR: import process is restarted by controller.",
		)

		// On a truly broken DVCR the re-import stalls in Pending/Failed instead of looping;
		// only a racing GC could cause Ready->ImageLost->recover churn every check interval.
		cvi.Status = v1alpha2.ClusterVirtualImageStatus{
			Phase:              v1alpha2.ImagePending,
			Conditions:         cvi.Status.Conditions,
			ObservedGeneration: cvi.Status.ObservedGeneration,
		}

		_, err := h.sources.CleanUp(ctx, cvi)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true}, nil
	}

	if cvi.Status.Phase == "" {
		cvi.Status.Phase = v1alpha2.ImagePending
	}

	dataSourceReadyCondition, exists := conditions.GetCondition(cvicondition.DatasourceReadyType, cvi.Status.Conditions)
	if !exists {
		return reconcile.Result{}, fmt.Errorf("condition %s not found, but required", cvicondition.DatasourceReadyType)
	}

	if dataSourceReadyCondition.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	if readyCondition.Status != metav1.ConditionTrue && h.sources.Changed(ctx, cvi) {
		cvi.Status = v1alpha2.ClusterVirtualImageStatus{
			Phase:              v1alpha2.ImagePending,
			Conditions:         cvi.Status.Conditions,
			ObservedGeneration: cvi.Status.ObservedGeneration,
		}

		_, err := h.sources.CleanUp(ctx, cvi)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true}, nil
	}

	ds, exists := h.sources.Get(cvi.Spec.DataSource.Type)
	if !exists {
		return reconcile.Result{}, fmt.Errorf("data source runner not found for type: %s", cvi.Spec.DataSource.Type)
	}

	result, err := ds.Sync(ctx, cvi)
	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}
