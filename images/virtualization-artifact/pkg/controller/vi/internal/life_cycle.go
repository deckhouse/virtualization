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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type LifeCycleHandler struct {
	client  client.Client
	sources *source.Sources
}

func NewLifeCycleHandler(sources *source.Sources, client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		client:  client,
		sources: sources,
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

	if vi.DeletionTimestamp != nil {
		vi.Status.Phase = virtv2.ImageTerminating
		return reconcile.Result{}, nil
	}

	if vi.Status.Phase == "" {
		vi.Status.Phase = virtv2.ImagePending
	}

	dataSourceReadyCondition, exists := conditions.GetCondition(vicondition.DatasourceReadyType, vi.Status.Conditions)
	if !exists {
		return reconcile.Result{}, fmt.Errorf("condition %s not found, but required", vicondition.DatasourceReadyType)
	}

	if dataSourceReadyCondition.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	if readyCondition.Status != metav1.ConditionTrue && h.sources.Changed(ctx, vi) {
		vi.Status = virtv2.VirtualImageStatus{
			ImageStatus: virtv2.ImageStatus{
				Phase: virtv2.ImagePending,
			},
			Conditions:         vi.Status.Conditions,
			ObservedGeneration: vi.Status.ObservedGeneration,
		}

		_, err := h.sources.CleanUp(ctx, vi)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true}, nil
	}

	ds, exists := h.sources.For(vi.Spec.DataSource.Type)
	if !exists {
		return reconcile.Result{}, fmt.Errorf("data source runner not found for type: %s", vi.Spec.DataSource.Type)
	}

	var result reconcile.Result
	var err error
	if vi.Spec.Storage == virtv2.StorageKubernetes {
		result, err = ds.StoreToPVC(ctx, vi)
	} else {
		result, err = ds.StoreToDVCR(ctx, vi)
	}

	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}

func (h LifeCycleHandler) Name() string {
	return "LifeCycleHandler"
}
