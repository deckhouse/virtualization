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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type DatasourceReadyHandler struct {
	sources *source.Sources
}

func NewDatasourceReadyHandler(sources *source.Sources) *DatasourceReadyHandler {
	return &DatasourceReadyHandler{
		sources: sources,
	}
}

func (h DatasourceReadyHandler) Handle(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(cvicondition.DatasourceReadyType).Generation(cvi.Generation)

	defer func() { conditions.SetCondition(cb, &cvi.Status.Conditions) }()

	if cvi.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	s, ok := h.sources.Get(cvi.Spec.DataSource.Type)
	if !ok {
		err := fmt.Errorf("data source validator not found for type: %s", cvi.Spec.DataSource.Type)
		cb.Message(err.Error())
		return reconcile.Result{}, err
	}

	err := s.Validate(ctx, cvi)
	switch {
	case err == nil:
		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.DatasourceReady).
			Message("")
		return reconcile.Result{}, nil
	case errors.Is(err, source.ErrSecretNotFound):
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.ContainerRegistrySecretNotFound).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	case errors.As(err, &source.ImageNotReadyError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.ImageNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	case errors.As(err, &source.ClusterImageNotReadyError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.ClusterImageNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	case errors.As(err, &source.VirtualDiskNotReadyError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.VirtualDiskNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	case errors.As(err, &source.VirtualDiskNotReadyForUseError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.VirtualDiskNotReadyForUse).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return reconcile.Result{}, nil
	case errors.As(err, &source.VirtualDiskAttachedToVirtualMachineError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.VirtualDiskAttachedToVirtualMachine).
			Message(service.CapitalizeFirstLetter(err.Error() + "."))
		return reconcile.Result{}, nil
	case errors.As(err, &source.VirtualDiskSnapshotNotReadyError{}):
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.VirtualDiskSnapshotNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, err
	}
}
