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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type LifeCycleHandler struct {
	client   client.Client
	blank    source.Handler
	sources  Sources
	recorder eventrecord.EventRecorderLogger
}

func NewLifeCycleHandler(recorder eventrecord.EventRecorderLogger, blank source.Handler, sources Sources, client client.Client) *LifeCycleHandler {
	return &LifeCycleHandler{
		client:   client,
		blank:    blank,
		sources:  sources,
		recorder: recorder,
	}
}

func (h LifeCycleHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	readyCondition, ok := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if !ok {
		readyCondition = metav1.Condition{
			Status: metav1.ConditionUnknown,
			Reason: conditions.ReasonUnknown.String(),
		}

		cb := conditions.NewConditionBuilder(vdcondition.ReadyType).
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Generation(vd.Generation)
		conditions.SetCondition(cb, &vd.Status.Conditions)
	}

	if vd.DeletionTimestamp != nil {
		vd.Status.Phase = virtv2.DiskTerminating
		return reconcile.Result{}, nil
	}

	if vd.Status.Phase == "" {
		vd.Status.Phase = virtv2.DiskPending
	}

	dataSourceReadyCondition, exists := conditions.GetCondition(vdcondition.DatasourceReadyType, vd.Status.Conditions)
	if !exists {
		return reconcile.Result{}, fmt.Errorf("condition %s not found, but required", vdcondition.DatasourceReadyType)
	}

	if dataSourceReadyCondition.Status != metav1.ConditionTrue {
		return reconcile.Result{}, nil
	}

	if readyCondition.Status != metav1.ConditionTrue && readyCondition.Reason != vdcondition.Lost.String() && h.sources.Changed(ctx, vd) {
		h.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVDStorageClassWasDeleted,
			"Spec changes are detected: restart import process",
		)

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

	storageClassReadyCondition, ok := conditions.GetCondition(vdcondition.StorageClassReadyType, vd.Status.Conditions)
	if !ok {
		return reconcile.Result{Requeue: true}, fmt.Errorf("condition %s not found", vdcondition.StorageClassReadyType)
	}

	if readyCondition.Status != metav1.ConditionTrue && storageClassReadyCondition.Status != metav1.ConditionTrue {
		readyCB := conditions.
			NewConditionBuilder(vdcondition.ReadyType).
			Generation(vd.Generation).
			Status(metav1.ConditionFalse).
			Reason(vdcondition.StorageClassNotReady).
			Message("Storage class is not ready, please read the StorageClassReady condition state.")

		conditions.SetCondition(readyCB, &vd.Status.Conditions)
	}

	if readyCondition.Status != metav1.ConditionTrue && storageClassReadyCondition.Status != metav1.ConditionTrue && vd.Status.StorageClassName != "" {
		h.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVDStorageClassWasDeleted,
			"Storage class was deleted while population",
		)

		vd.Status = virtv2.VirtualDiskStatus{
			Phase:              virtv2.DiskPending,
			Conditions:         vd.Status.Conditions,
			ObservedGeneration: vd.Status.ObservedGeneration,
			StorageClassName:   vd.Status.StorageClassName,
		}

		_, err := h.sources.CleanUp(ctx, vd)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to clean up to restart import process: %w", err)
		}

		return reconcile.Result{}, nil
	}

	if vd.Status.StorageClassName != "" && storageClassReadyCondition.Status == metav1.ConditionTrue {
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

	return reconcile.Result{}, nil
}
