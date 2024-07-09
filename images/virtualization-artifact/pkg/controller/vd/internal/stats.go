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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type StatsHandler struct{}

func NewStatsHandler() *StatsHandler {
	return &StatsHandler{}
}

func (h StatsHandler) Handle(_ context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	sinceCreation := time.Since(vd.CreationTimestamp.Time).Truncate(time.Second)

	datasourceReady, _ := service.GetCondition(vdcondition.DatasourceReadyType, vd.Status.Conditions)
	if datasourceReady.Status == metav1.ConditionTrue && vd.Status.Stats.CreationDuration.WaitingForDependencies == nil {
		vd.Status.Stats.CreationDuration.WaitingForDependencies = &metav1.Duration{
			Duration: sinceCreation,
		}
	}

	ready, _ := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	if ready.Status == metav1.ConditionTrue && vd.Status.Stats.CreationDuration.Provisioning == nil {
		duration := sinceCreation

		if vd.Status.Stats.CreationDuration.WaitingForDependencies != nil {
			duration -= vd.Status.Stats.CreationDuration.WaitingForDependencies.Duration
		}

		vd.Status.Stats.CreationDuration.Provisioning = &metav1.Duration{
			Duration: duration,
		}
	}

	return reconcile.Result{}, nil
}
