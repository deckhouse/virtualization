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
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
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

func (h LifeCycleHandler) Handle(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	readyCondition, ok := service.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	if !ok {
		readyCondition = metav1.Condition{
			Type:   cvicondition.ReadyType,
			Status: metav1.ConditionUnknown,
			Reason: conditions.ReasonUnknown.String(),
		}

		service.SetCondition(readyCondition, &cvi.Status.Conditions)
	}

	if cvi.DeletionTimestamp != nil {
		cvi.Status.Phase = virtv2.ImageTerminating
		return reconcile.Result{}, nil
	}

	if cvi.Status.Phase == "" {
		cvi.Status.Phase = virtv2.ImagePending
	}

	dataSourceReadyCondition, exists := service.GetCondition(cvicondition.DatasourceReadyType, cvi.Status.Conditions)
	if !exists {
		return reconcile.Result{}, fmt.Errorf("condition %s not found, but required", cvicondition.DatasourceReadyType)
	}

	if dataSourceReadyCondition.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	if readyCondition.Status != metav1.ConditionTrue && h.sources.Changed(ctx, cvi) {
		cvi.Status = virtv2.ClusterVirtualImageStatus{
			ImageStatus: virtv2.ImageStatus{
				Phase: virtv2.ImagePending,
			},
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
