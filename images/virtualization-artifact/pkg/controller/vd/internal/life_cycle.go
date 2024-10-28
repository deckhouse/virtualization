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

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type LifeCycleHandler struct {
	client  client.Client
	blank   source.Handler
	sources *source.Sources
}

func NewLifeCycleHandler(blank source.Handler, sources *source.Sources, client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		client:  client,
		blank:   blank,
		sources: sources,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("lifecycle"))

	readyCondition, ok := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !ok {
		readyCondition = metav1.Condition{
			Status: metav1.ConditionUnknown,
		}
	}

	if vd.DeletionTimestamp != nil {
		vd.Status.Phase = virtv2.DiskTerminating
		return reconcile.Result{}, nil
	}

	if vd.Status.Phase == "" {
		vd.Status.Phase = virtv2.DiskPending
	}

	dataSourceReadyCondition, exists := service.GetCondition(vdcondition.DatasourceReadyType, vd.Status.Conditions)
	if !exists {
		return reconcile.Result{}, fmt.Errorf("condition %s not found, but required", vdcondition.DatasourceReadyType)
	}

	if dataSourceReadyCondition.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	if readyCondition.Status != metav1.ConditionTrue && readyCondition.Reason != vdcondition.Lost && h.sources.Changed(ctx, vd) {
		log.Info("Spec changes are detected: restart import process")

		vd.Status = virtv2.VirtualDiskStatus{
			Phase:              virtv2.DiskPending,
			Conditions:         vd.Status.Conditions,
			ObservedGeneration: vd.Status.ObservedGeneration,
		}

		_, err := h.sources.CleanUp(ctx, vd)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to clean up to restart import process: %w", err)
		}

		return reconcile.Result{Requeue: true}, nil
	}

	var ds source.Handler
	if vd.Spec.DataSource == nil {
		ds = h.blank
	} else {
		ds, exists = h.sources.Get(vd.Spec.DataSource.Type)
		if !exists {
			return reconcile.Result{}, fmt.Errorf("data source runner not found for type: %s", vd.Spec.DataSource.Type)
		}
	}

	result, err := ds.Sync(ctx, vd)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync virtual disk data source %s: %w", ds.Name(), err)
	}

	return result, nil
}
