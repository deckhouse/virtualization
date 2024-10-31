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

package source

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type Handler interface {
	Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error)
	CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error)
	Validate(ctx context.Context, cvi *virtv2.ClusterVirtualImage) error
}

type Sources struct {
	sources map[virtv2.DataSourceType]Handler
}

func NewSources() *Sources {
	return &Sources{
		sources: make(map[virtv2.DataSourceType]Handler),
	}
}

func (s Sources) Set(dsType virtv2.DataSourceType, h Handler) {
	s.sources[dsType] = h
}

func (s Sources) Get(dsType virtv2.DataSourceType) (Handler, bool) {
	source, ok := s.sources[dsType]
	return source, ok
}

func (s Sources) Changed(_ context.Context, cvi *virtv2.ClusterVirtualImage) bool {
	return cvi.Generation != cvi.Status.ObservedGeneration
}

func (s Sources) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	var mergedResult reconcile.Result

	for _, source := range s.sources {
		result, err := source.CleanUp(ctx, cvi)
		if err != nil {
			return reconcile.Result{}, err
		}

		mergedResult = service.MergeResults(mergedResult, result)
	}

	return mergedResult, nil
}

type Cleaner interface {
	CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error)
}

func CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage, c Cleaner) (reconcile.Result, error) {
	if cc.ShouldCleanupSubResources(cvi) {
		return c.CleanUp(ctx, cvi)
	}

	return reconcile.Result{}, nil
}

func isDiskProvisioningFinished(c metav1.Condition) bool {
	return c.Reason == cvicondition.Ready.String()
}

const retryPeriod = 1

func setQuotaExceededPhaseCondition(cb *conditions.ConditionBuilder, phase *virtv2.ImagePhase, err error, creationTimestamp metav1.Time) reconcile.Result {
	*phase = virtv2.ImageFailed
	cb.Status(metav1.ConditionFalse).
		Reason(cvicondition.ProvisioningFailed)

	if creationTimestamp.Add(30 * time.Minute).After(time.Now()) {
		cb.Message(fmt.Sprintf("Quota exceeded: %s; Please configure quotas or try recreating the resource later.", err))
		return reconcile.Result{}
	}

	cb.Message(fmt.Sprintf("Quota exceeded: %s; Retry in %d minute.", err, retryPeriod))
	return reconcile.Result{RequeueAfter: retryPeriod * time.Minute}
}

func setPhaseConditionToFailed(readyCB *conditions.ConditionBuilder, phase *virtv2.ImagePhase, err error) {
	*phase = virtv2.ImageFailed
	readyCB.Status(metav1.ConditionFalse).
		Reason(cvicondition.ProvisioningFailed).
		Message(service.CapitalizeFirstLetter(err.Error()))
}
