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

	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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

func (h DatasourceReadyHandler) Handle(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	condition, ok := service.GetCondition(cvicondition.DatasourceReadyType, cvi.Status.Conditions)
	if !ok {
		condition = metav1.Condition{
			Type:   cvicondition.DatasourceReadyType,
			Status: metav1.ConditionUnknown,
		}
	}

	defer func() { service.SetCondition(condition, &cvi.Status.Conditions) }()

	if cvi.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	s, ok := h.sources.For(cvi.Spec.DataSource.Type)
	if !ok {
		err := fmt.Errorf("data source validator not found for type: %s", cvi.Spec.DataSource.Type)
		condition.Message = err.Error()
		return reconcile.Result{}, err
	}

	err := s.Validate(ctx, cvi)
	switch {
	case err == nil:
		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.DatasourceReadyReason_DatasourceReady
		condition.Message = ""
	case errors.Is(err, source.ErrSecretNotFound):
		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.DatasourceReadyReason_ContainerRegistrySecretNotFound
		condition.Message = strings.ToTitle(err.Error())
	case errors.As(err, &source.ImageNotReadyError{}):
		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.DatasourceReadyReason_ImageNotReady
		condition.Message = strings.ToTitle(err.Error())
	case errors.As(err, &source.ClusterImageNotReadyError{}):
		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.DatasourceReadyReason_ClusterImageNotReady
		condition.Message = strings.ToTitle(err.Error())
	}

	return reconcile.Result{}, err
}
