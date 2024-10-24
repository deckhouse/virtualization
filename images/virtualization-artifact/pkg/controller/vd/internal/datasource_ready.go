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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type DatasourceReadyHandler struct {
	sources Sources
	blank   source.Handler
}

func NewDatasourceReadyHandler(blank source.Handler, sources Sources) *DatasourceReadyHandler {
	return &DatasourceReadyHandler{
		blank:   blank,
		sources: sources,
	}
}

func (h DatasourceReadyHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	condition := metav1.Condition{
		Type:   vdcondition.DatasourceReadyType,
		Status: metav1.ConditionUnknown,
	}

	defer func() { service.SetCondition(condition, &vd.Status.Conditions) }()

	if vd.DeletionTimestamp != nil {
		condition.Status = metav1.ConditionUnknown
		condition.Reason = ""
		condition.Message = ""
		return reconcile.Result{}, nil
	}

	var ds source.Handler
	var ok bool
	if vd.Spec.DataSource == nil {
		ds = h.blank
	} else {
		ds, ok = h.sources.Get(vd.Spec.DataSource.Type)
		if !ok {
			return reconcile.Result{}, fmt.Errorf("data source validator not found for type: %s", vd.Spec.DataSource.Type)
		}
	}

	err := ds.Validate(ctx, vd)
	switch {
	case err == nil:
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.DatasourceReady
		condition.Message = ""
		return reconcile.Result{}, nil
	case errors.Is(err, source.ErrSecretNotFound):
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ContainerRegistrySecretNotFound
		condition.Message = service.CapitalizeFirstLetter(err.Error()) + "."
		return reconcile.Result{}, nil
	case errors.As(err, &source.ImageNotReadyError{}):
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ImageNotReady
		condition.Message = service.CapitalizeFirstLetter(err.Error()) + "."
		return reconcile.Result{}, nil
	case errors.As(err, &source.ClusterImageNotReadyError{}):
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ClusterImageNotReady
		condition.Message = service.CapitalizeFirstLetter(err.Error()) + "."
		return reconcile.Result{}, nil
	case errors.As(err, &source.VirtualDiskSnapshotNotReadyError{}):
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.VirtualDiskSnapshotNotReady
		condition.Message = service.CapitalizeFirstLetter(err.Error()) + "."
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, fmt.Errorf("validation failed for data source %s: %w", ds.Name(), err)
	}
}
