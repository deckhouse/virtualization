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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const uploadDataSource = "upload"

type UploadDataSource struct {
	statService     UploadDataSourceStatService
	uploaderService UploadDataSourceUploaderService
	diskService     UploadDataSourceDiskService
	dvcrSettings    *dvcr.Settings
	recorder        eventrecord.EventRecorderLogger
	client          client.Client
}

func NewUploadDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService UploadDataSourceStatService,
	uploaderService UploadDataSourceUploaderService,
	diskService UploadDataSourceDiskService,
	dvcrSettings *dvcr.Settings,
	client client.Client,
) *UploadDataSource {
	return &UploadDataSource{
		statService:     statService,
		uploaderService: uploaderService,
		diskService:     diskService,
		dvcrSettings:    dvcrSettings,
		client:          client,
		recorder:        recorder,
	}
}

func (ds UploadDataSource) Sync(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, uploadDataSource)

	supgen := vdsupplements.NewGenerator(vd)

	cb := conditions.NewConditionBuilder(vdcondition.ReadyType).Generation(vd.Generation)
	defer func() { conditions.SetCondition(cb, &vd.Status.Conditions) }()

	pod, err := ds.uploaderService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch uploader pod: %w", err)
	}
	svc, err := ds.uploaderService.GetService(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch uploader service: %w", err)
	}
	ing, err := ds.uploaderService.GetIngress(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch uploader ingress: %w", err)
	}

	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc: %w", err)
	}
	if pvc != nil {
		ctx = logger.ToContext(ctx, log.With("pvc.name", pvc.Name, "pvc.status.phase", pvc.Status.Phase))
	}

	return steptaker.NewStepTakers[*v1alpha2.VirtualDisk](
		step.NewCleanUpUploaderStep(pod, svc, ing, ds.uploaderService),
		step.NewReadyStep(ds.diskService, pvc, cb),
		step.NewTerminatingStep(pvc),
		step.NewCreateUploaderStep(pvc, pod, svc, ing, ds.uploaderService, ds.dvcrSettings, ds.recorder, cb),
		step.NewWaitForUserUploadStep(pod, svc, ing, ds.statService, ds.uploaderService, ds.client, cb),
		step.NewPVCImportFromDVCRStep(pvc, pod, ds.statService, ds.diskService, ds.client, ds.recorder, cb, "The Upload DataSource import to PVC has started"),
		step.NewWaitForPVCStep(pvc, ds.client, cb),
		step.NewWaitForPVCImportStep(pvc, step.DVCRPodPVCImportSource(pod, ds.statService), ds.diskService, ds.client, cb),
	).Run(ctx, vd)
}

func (ds UploadDataSource) CleanUp(ctx context.Context, vd *v1alpha2.VirtualDisk) (bool, error) {
	supgen := vdsupplements.NewGenerator(vd)

	uploaderRequeue, err := ds.uploaderService.CleanUp(ctx, supgen)
	if err != nil {
		return false, fmt.Errorf("clean up uploader: %w", err)
	}

	diskRequeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, fmt.Errorf("clean up disk: %w", err)
	}

	return uploaderRequeue || diskRequeue, nil
}

func (ds UploadDataSource) Validate(_ context.Context, _ *v1alpha2.VirtualDisk) error {
	return nil
}

func (ds UploadDataSource) Name() string {
	return uploadDataSource
}
