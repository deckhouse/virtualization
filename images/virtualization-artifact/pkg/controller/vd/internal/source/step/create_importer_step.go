/*
Copyright 2026 Flant JSC

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

package step

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type CreateImporterStepImporterService interface {
	Start(ctx context.Context, settings *importer.Settings, obj client.Object, sup supplements.Generator, caBundle *datasource.CABundle, opts ...service.Option) error
}

// ImporterEnvSettingsBuilder builds importer environment settings for a given
// VirtualDisk. Data sources provide their source-specific build logic via this
// callback so the step itself stays generic.
type ImporterEnvSettingsBuilder func(vd *v1alpha2.VirtualDisk, supgen supplements.Generator) *importer.Settings

// CreateImporterStep creates an importer Pod that downloads source data into
// DVCR. It is a no-op once the underlying PVC has been created so the importer
// is not recreated after cleanup.
type CreateImporterStep struct {
	pvc             *corev1.PersistentVolumeClaim
	pod             *corev1.Pod
	settingsBuilder ImporterEnvSettingsBuilder
	importer        CreateImporterStepImporterService
	recorder        eventrecord.EventRecorderLogger
	cb              *conditions.ConditionBuilder
	eventText       string
}

func NewCreateImporterStep(
	pvc *corev1.PersistentVolumeClaim,
	pod *corev1.Pod,
	settingsBuilder ImporterEnvSettingsBuilder,
	importer CreateImporterStepImporterService,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
	eventText string,
) *CreateImporterStep {
	return &CreateImporterStep{
		pvc:             pvc,
		pod:             pod,
		settingsBuilder: settingsBuilder,
		importer:        importer,
		recorder:        recorder,
		cb:              cb,
		eventText:       eventText,
	}
}

func (s CreateImporterStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	// Importer is needed only until the underlying PVC has been created.
	// Once the PVC exists the data has been pushed to DVCR and the importer
	// supplements are not recreated even if they are missing.
	if s.pvc != nil {
		return nil, nil
	}

	if s.pod != nil {
		return nil, nil
	}

	s.recorder.Event(
		vd,
		corev1.EventTypeNormal,
		v1alpha2.ReasonDataSourceSyncStarted,
		s.eventText,
	)

	vd.Status.Progress = "0%"

	supgen := vdsupplements.NewGenerator(vd)
	settings := s.settingsBuilder(vd, supgen)
	caBundle := datasource.NewCABundleForVMD(vd.GetNamespace(), vd.Spec.DataSource)

	err := s.importer.Start(ctx, settings, vd, supgen, caBundle, service.WithSystemNodeToleration())
	switch {
	case err == nil:
		// OK.
	case common.ErrQuotaExceeded(err):
		s.recorder.Event(vd, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.QuotaExceeded).
			Message(fmt.Sprintf("Quota exceeded during the importer provisioning: %s", err))
		return &reconcile.Result{}, nil
	default:
		return nil, fmt.Errorf("start importer: %w", err)
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("DVCR Provisioner not found: create the new one.")

	return &reconcile.Result{Requeue: true}, nil
}
