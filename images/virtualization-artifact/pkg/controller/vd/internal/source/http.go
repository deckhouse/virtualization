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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const httpDataSource = "http"

type HTTPDataSource struct {
	statService     HTTPDataSourceStatService
	importerService HTTPDataSourceImporterService
	diskService     HTTPDataSourceDiskService
	dvcrSettings    *dvcr.Settings
	client          client.Client
	recorder        eventrecord.EventRecorderLogger
}

func NewHTTPDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService HTTPDataSourceStatService,
	importerService HTTPDataSourceImporterService,
	diskService HTTPDataSourceDiskService,
	dvcrSettings *dvcr.Settings,
	client client.Client,
) *HTTPDataSource {
	return &HTTPDataSource{
		statService:     statService,
		importerService: importerService,
		diskService:     diskService,
		dvcrSettings:    dvcrSettings,
		client:          client,
		recorder:        recorder,
	}
}

func (ds HTTPDataSource) Sync(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, httpDataSource)

	supgen := vdsupplements.NewGenerator(vd)

	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch importer pod: %w", err)
	}

	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc: %w", err)
	}
	if pvc != nil {
		ctx = logger.ToContext(ctx, log.With("pvc.name", pvc.Name, "pvc.status.phase", pvc.Status.Phase))
	}

	return steptaker.NewStepTakers[*v1alpha2.VirtualDisk](
		step.NewCleanUpImporterStep(pod, ds.importerService),
		step.NewReadyStep(ds.diskService, pvc, cb),
		step.NewTerminatingStep(pvc),
		step.NewCreateImporterStep(pvc, pod, ds.buildEnvSettings, ds.importerService, ds.recorder, cb, "The HTTP DataSource import to DVCR has started"),
		step.NewWaitForDVCRImporterStep(pod, ds.statService, ds.importerService, ds.client, cb),
		step.NewPVCImportFromDVCRStep(pvc, pod, ds.statService, ds.diskService, ds.client, ds.recorder, cb, "The HTTP DataSource import to PVC has started"),
		step.NewWaitForPVCStep(pvc, ds.client, cb),
		step.NewWaitForPVCImportStep(pvc, step.DVCRPodPVCImportSource(pod, ds.statService), ds.diskService, ds.client, cb),
	).Run(ctx, vd)
}

func (ds HTTPDataSource) CleanUp(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	supgen := vdsupplements.NewGenerator(vd)

	importerRequeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, fmt.Errorf("clean up importer: %w", err)
	}

	diskRequeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, fmt.Errorf("clean up disk: %w", err)
	}

	return importerRequeue || diskRequeue, nil
}

func (ds HTTPDataSource) Validate(_ context.Context, _ *v1alpha2.VirtualDisk) error {
	return nil
}

func (ds HTTPDataSource) Name() string {
	return httpDataSource
}

func (ds HTTPDataSource) buildEnvSettings(vd *v1alpha2.VirtualDisk, supgen supplements.Generator) *importer.Settings {
	var settings importer.Settings

	importer.ApplyHTTPSourceSettings(&settings, vd.Spec.DataSource.HTTP, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVD(vd),
	)

	return &settings
}
