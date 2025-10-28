/*
Copyright 2025 Flant JSC

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

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LifeCycleHandler struct {
	client client.Client
}

func NewLifeCycleHandler(client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		client: client,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, deploy *appsv1.Deployment) (reconcile.Result, error) {
	// Detect event source: cron, deployment, or vi/cvi/vd status change.
	// Get secret
	// Get deployment, detect maintenance mode and readiness.

	// If wait for vi/cvi/vd, list them and detect the moment when there are no resources in Provisioning state.

	// Switch to the net state: Update secret annotation/Delete secret/ deployment

	return reconcile.Result{}, nil

	/*
		maintenanceCondition, ok := conditions.GetCondition(cvicondition.ReadyType, deploy.Status.Conditions)
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
	*/
}
