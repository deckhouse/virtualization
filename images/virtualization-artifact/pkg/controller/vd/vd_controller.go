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

package vd

import (
	"context"
	"log/slog"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unsafe"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	vdcolelctor "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vd"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "vd-controller"

	PodVerbose    = "3"
	PodPullPolicy = string(corev1.PullIfNotPresent)
)

type Condition interface {
	Handle(ctx context.Context, vd *virtv2.VirtualDisk) error
}

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	importerImage string,
	uploaderImage string,
	dvcr *dvcr.Settings,
) (controller.Controller, error) {
	logger := slog.Default().With("controller", ControllerName)

	stat := service.NewStatService()
	protection := service.NewProtectionService(mgr.GetClient(), virtv2.FinalizerVDProtection)
	importer := service.NewImporterService(dvcr, mgr.GetClient(), importerImage, PodPullPolicy, PodVerbose, ControllerName, protection)
	uploader := service.NewUploaderService(dvcr, mgr.GetClient(), uploaderImage, PodPullPolicy, PodVerbose, ControllerName, protection)
	disk := service.NewDiskService(mgr.GetClient(), dvcr, protection)

	blank := source.NewBlankDataSource(stat, disk, logger)

	sources := source.NewSources()
	sources.Set(virtv2.DataSourceTypeHTTP, source.NewHTTPDataSource(stat, importer, disk, dvcr, logger))
	sources.Set(virtv2.DataSourceTypeContainerImage, source.NewRegistryDataSource(stat, importer, disk, dvcr, mgr.GetClient(), logger))
	sources.Set(virtv2.DataSourceTypeObjectRef, source.NewObjectRefDataSource(stat, disk, mgr.GetClient(), logger))
	sources.Set(virtv2.DataSourceTypeUpload, source.NewUploadDataSource(stat, uploader, disk, dvcr, logger))

	reconciler := NewReconciler(
		mgr.GetClient(),
		logger,
		internal.NewDatasourceReadyHandler(blank, sources),
		internal.NewLifeCycleHandler(logger, blank, sources, mgr.GetClient()),
		internal.NewResizingHandler(logger, disk),
		internal.NewDeletionHandler(sources),
		internal.NewAttacheeHandler(mgr.GetClient()),
		internal.NewStatsHandler(stat, importer, uploader),
	)

	vdController, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, vdController)
	if err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&virtv2.VirtualDisk{}).
		WithValidator(NewValidator()).
		Complete(); err != nil {
		return nil, err
	}

	vdcolelctor.SetupCollector(&lister{uc: unsafe.NewClient(mgr.GetCache())}, metrics.Registry)

	log.Info("Initialized VirtualDisk controller", "image", importerImage)

	return vdController, nil
}

type lister struct {
	uc unsafe.Client
}

func (l lister) List(ctx context.Context) ([]virtv2.VirtualDisk, error) {
	disks := virtv2.VirtualDiskList{}
	err := l.uc.List(ctx, &disks)
	if err != nil {
		return nil, err
	}
	return disks.Items, nil
}
