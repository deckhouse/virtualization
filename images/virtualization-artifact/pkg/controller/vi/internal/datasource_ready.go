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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type DatasourceReadyHandler struct {
	sources *source.Sources
}

func NewDatasourceReadyHandler(sources *source.Sources) *DatasourceReadyHandler {
	return &DatasourceReadyHandler{
		sources: sources,
	}
}

func (h DatasourceReadyHandler) Handle(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error) {
	condition, ok := service.GetCondition(vicondition.DatasourceReadyType, vi.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   vicondition.DatasourceReadyType,
			Status: metav1.ConditionUnknown,
		}
	}

	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	if vi.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	s, ok := h.sources.For(vi.Spec.DataSource.Type)
	if !ok {
		err := fmt.Errorf("data source validator not found for type: %s", vi.Spec.DataSource.Type)
		condition.Message = err.Error()
		return reconcile.Result{}, err
	}

	err := s.Validate(ctx, vi)
	switch {
	case err == nil:
		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.DatasourceReady
		condition.Message = ""
	case errors.Is(err, source.ErrSecretNotFound):
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.ContainerRegistrySecretNotFound
		condition.Message = strings.ToTitle(err.Error())
	case errors.As(err, &source.ImageNotReadyError{}):
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.ImageNotReady
		condition.Message = strings.ToTitle(err.Error())
	}

	return reconcile.Result{}, err
}

func (h DatasourceReadyHandler) Name() string {
	return "DatasourceReadyHandler"
}
