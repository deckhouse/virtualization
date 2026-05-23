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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/steptaker"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const registryDataSource = "registry"

type RegistryDataSource struct {
	statService     RegistryDataSourceStatService
	importerService RegistryDataSourceImporterService
	diskService     RegistryDataSourceDiskService
	pvcService      DataSourcePVCService
	dvcrSettings    *dvcr.Settings
	client          client.Client
	recorder        eventrecord.EventRecorderLogger
}

func NewRegistryDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService RegistryDataSourceStatService,
	importerService RegistryDataSourceImporterService,
	diskService RegistryDataSourceDiskService,
	pvcService DataSourcePVCService,
	dvcrSettings *dvcr.Settings,
	client client.Client,
) *RegistryDataSource {
	return &RegistryDataSource{
		statService:     statService,
		importerService: importerService,
		diskService:     diskService,
		pvcService:      pvcService,
		dvcrSettings:    dvcrSettings,
		client:          client,
		recorder:        recorder,
	}
}

func (ds RegistryDataSource) Sync(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, registryDataSource)

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
		step.NewCreateImporterStep(pvc, pod, ds.buildEnvSettings, ds.importerService, ds.recorder, cb, "The Registry DataSource import to DVCR has started"),
		step.NewWaitForDVCRImporterStep(pod, ds.statService, ds.importerService, ds.client, cb),
		step.NewPVCImportFromDVCRStep(pvc, pod, ds.statService, ds.diskService, ds.pvcService, ds.client, ds.recorder, cb, "The Registry DataSource import to PVC has started"),
		step.NewWaitForPVCStep(pvc, ds.client, cb),
		step.NewWaitForPVCImportStep(pvc, step.DVCRPodPVCImportSource(pod, ds.statService), ds.pvcService, ds.statService, service.NewScaleOption(50, 100), ds.client, cb),
	).Run(ctx, vd)
}

func (ds RegistryDataSource) CleanUp(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
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

func (ds RegistryDataSource) Validate(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ContainerImage == nil {
		return errors.New("container image missed for data source")
	}

	if vd.Spec.DataSource.ContainerImage.ImagePullSecret.Name != "" {
		secretName := types.NamespacedName{
			Namespace: vd.GetNamespace(),
			Name:      vd.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
		}
		secret, err := object.FetchObject[*corev1.Secret](ctx, secretName, ds.client, &corev1.Secret{})
		if err != nil {
			return fmt.Errorf("failed to get secret %s: %w", secretName, err)
		}

		if secret == nil {
			return ErrSecretNotFound
		}
	}

	return nil
}

func (ds RegistryDataSource) Name() string {
	return registryDataSource
}

func (ds RegistryDataSource) buildEnvSettings(vd *v1alpha2.VirtualDisk, supgen supplements.Generator) *importer.Settings {
	var settings importer.Settings

	containerImage := &datasource.ContainerRegistry{
		Image: vd.Spec.DataSource.ContainerImage.Image,
		ImagePullSecret: types.NamespacedName{
			Name:      vd.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
			Namespace: vd.GetNamespace(),
		},
	}
	importer.ApplyRegistrySourceSettings(&settings, containerImage, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForVD(vd),
	)

	return &settings
}
