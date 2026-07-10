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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
		return reconcile.Result{}, nil
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

	normalizeProgress(cvi)

	return result, nil
}

// normalizeProgress enforces the phase/progress invariants on the final status:
//   - a CVI that has not yet entered Provisioning ("" or Pending) must NOT
//     expose any progress percentage. Progress describes how far the import
//     has advanced, so it is meaningful only once the import has actually
//     started; any earlier setter (some source paths optimistically populate
//     "0%" before they have decided whether the phase will advance to
//     Provisioning in this reconcile) is unconditionally cleared here so that
//     consumers never observe e.g. "Pending 0%";
//   - a CVI in the Provisioning phase must always expose a progress
//     percentage; until the importer reports real progress it defaults to
//     "0%";
//   - a CVI parked in WaitForUserUpload has not received any data yet, so
//     its progress is always "0%".
func normalizeProgress(cvi *v1alpha2.ClusterVirtualImage) {
	switch cvi.Status.Phase {
	case "", v1alpha2.ImagePending:
		cvi.Status.Progress = ""
	case v1alpha2.ImageProvisioning:
		if cvi.Status.Progress == "" {
			cvi.Status.Progress = "0%"
		}
	case v1alpha2.ImageWaitForUserUpload:
		cvi.Status.Progress = "0%"
	}
}
