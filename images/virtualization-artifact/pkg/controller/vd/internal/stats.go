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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type StatsHandler struct {
	stat     *service.StatService
	importer *service.ImporterService
	uploader *service.UploaderService
}

func NewStatsHandler(stat *service.StatService, importer *service.ImporterService, uploader *service.UploaderService) *StatsHandler {
	return &StatsHandler{
		stat:     stat,
		importer: importer,
		uploader: uploader,
	}
}

func (h StatsHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	sinceCreation := time.Since(vd.CreationTimestamp.Time).Truncate(time.Second)

	readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	var isDatasourceReady bool
	if source.IsDiskProvisioningFinished(readyCondition) {
		isDatasourceReady = true
	} else {
		datasourceReadyCondition, _ := conditions.GetCondition(vdcondition.DatasourceReadyType, vd.Status.Conditions)
		isDatasourceReady = datasourceReadyCondition.Status == metav1.ConditionTrue && conditions.IsLastUpdated(datasourceReadyCondition, vd)
	}

	if isDatasourceReady &&
		vd.Status.Stats.CreationDuration.WaitingForDependencies == nil {
		vd.Status.Stats.CreationDuration.WaitingForDependencies = &metav1.Duration{
			Duration: sinceCreation,
		}
	}

	if readyCondition.Status == metav1.ConditionTrue &&
		conditions.IsLastUpdated(readyCondition, vd) &&
		vd.Status.Stats.CreationDuration.TotalProvisioning == nil {
		duration := sinceCreation

		if vd.Status.Stats.CreationDuration.WaitingForDependencies != nil {
			duration -= vd.Status.Stats.CreationDuration.WaitingForDependencies.Duration
		}

		vd.Status.Stats.CreationDuration.TotalProvisioning = &metav1.Duration{
			Duration: duration,
		}
	}

	if vd.Spec.DataSource == nil {
		return reconcile.Result{}, nil
	}

	supgen := vdsupplements.NewGenerator(vd)

	var pod *corev1.Pod
	var err error

	switch vd.Spec.DataSource.Type {
	case v1alpha2.DataSourceTypeUpload:
		pod, err = h.uploader.GetPod(ctx, supgen.Generator)
		if err != nil {
			return reconcile.Result{}, err
		}
	default:
		pod, err = h.importer.GetPod(ctx, supgen.Generator)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if pod == nil {
		return reconcile.Result{}, nil
	}

	vd.Status.Stats.CreationDuration.DVCRProvisioning = &metav1.Duration{
		Duration: h.stat.GetImportDuration(pod),
	}

	return reconcile.Result{}, nil
}
