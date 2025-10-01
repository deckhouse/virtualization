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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type DatasourceReadyHandler struct {
	sources  Sources
	blank    source.Handler
	recorder eventrecord.EventRecorderLogger
}

func NewDatasourceReadyHandler(recorder eventrecord.EventRecorderLogger, blank source.Handler, sources Sources) *DatasourceReadyHandler {
	return &DatasourceReadyHandler{
		blank:    blank,
		sources:  sources,
		recorder: recorder,
	}
}

func (h DatasourceReadyHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	if vd.DeletionTimestamp != nil {
		conditions.RemoveCondition(vdcondition.DatasourceReadyType, &vd.Status.Conditions)
		return reconcile.Result{}, nil
	}

	readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if source.IsDiskProvisioningFinished(readyCondition) {
		conditions.RemoveCondition(vdcondition.DatasourceReadyType, &vd.Status.Conditions)
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vdcondition.DatasourceReadyType).Generation(vd.Generation)

	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	var ds source.Handler
	if vd.Spec.DataSource == nil {
		ds = h.blank
	} else {
		var ok bool
		ds, ok = h.sources.Get(vd.Spec.DataSource.Type)
		if !ok {
			return reconcile.Result{}, fmt.Errorf("data source validator not found for type: %s", vd.Spec.DataSource.Type)
		}
	}

	err := ds.Validate(ctx, vd)
	switch {
	case err == nil:
		cb.
			Status(metav1.ConditionTrue).
			Reason(vdcondition.DatasourceReady).
			Message("")
		return reconcile.Result{}, nil
	case errors.Is(err, source.ErrSecretNotFound):
		h.recorder.Event(
			vd,
			corev1.EventTypeNormal,
			v1alpha2.ReasonVDContainerRegistrySecretNotFound,
			"Container registry secret not found",
		)

		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ContainerRegistrySecretNotFound).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, nil
	case errors.As(err, &source.ImageNotReadyError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ImageNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, nil
	case errors.As(err, &source.ClusterImageNotReadyError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ClusterImageNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, nil
	case errors.As(err, &source.VirtualDiskSnapshotNotReadyError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.VirtualDiskSnapshotNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, nil
	case errors.As(err, &source.ImageNotFoundError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ImageNotFound).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, nil
	case errors.As(err, &source.ClusterImageNotFoundError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.ClusterImageNotFound).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, fmt.Errorf("validation failed for data source %s: %w", ds.Name(), err)
	}
}
