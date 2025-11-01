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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	vdcolelctor "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vd"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "vd-controller"

	PodVerbose    = "3"
	PodPullPolicy = string(corev1.PullIfNotPresent)
)

type Condition interface {
	Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) error
}

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	importerImage string,
	uploaderImage string,
	requirements corev1.ResourceRequirements,
	dvcr *dvcr.Settings,
	storageClassSettings config.VirtualDiskStorageClassSettings,
) (controller.Controller, error) {
	stat := service.NewStatService(log)
	protection := service.NewProtectionService(mgr.GetClient(), v1alpha2.FinalizerVDProtection)
	importer := service.NewImporterService(dvcr, mgr.GetClient(), importerImage, requirements, PodPullPolicy, PodVerbose, ControllerName, protection)
	uploader := service.NewUploaderService(dvcr, mgr.GetClient(), uploaderImage, requirements, PodPullPolicy, PodVerbose, ControllerName, protection)
	disk := service.NewDiskService(mgr.GetClient(), dvcr, protection, ControllerName)
	scService := intsvc.NewVirtualDiskStorageClassService(service.NewBaseStorageClassService(mgr.GetClient()), storageClassSettings)
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)

	blank := source.NewBlankDataSource(recorder, disk, mgr.GetClient())

	sources := source.NewSources()
	sources.Set(v1alpha2.DataSourceTypeHTTP, source.NewHTTPDataSource(recorder, stat, importer, disk, dvcr, mgr.GetClient()))
	sources.Set(v1alpha2.DataSourceTypeContainerImage, source.NewRegistryDataSource(recorder, stat, importer, disk, dvcr, mgr.GetClient()))
	sources.Set(v1alpha2.DataSourceTypeObjectRef, source.NewObjectRefDataSource(recorder, disk, mgr.GetClient()))
	sources.Set(v1alpha2.DataSourceTypeUpload, source.NewUploadDataSource(recorder, stat, uploader, disk, dvcr, mgr.GetClient()))

	reconciler := NewReconciler(
		mgr.GetClient(),
		internal.NewInitHandler(),
		internal.NewStorageClassReadyHandler(scService),
		internal.NewDatasourceReadyHandler(recorder, blank, sources),
		internal.NewLifeCycleHandler(recorder, blank, sources, mgr.GetClient()),
		internal.NewSnapshottingHandler(disk),
		internal.NewResizingHandler(recorder, disk),
		internal.NewDeletionHandler(sources, mgr.GetClient()),
		internal.NewStatsHandler(stat, importer, uploader),
		internal.NewInUseHandler(mgr.GetClient()),
		internal.NewMigrationHandler(mgr.GetClient(), scService, disk, featuregates.Default()),
		internal.NewProtectionHandler(),
	)

	vdController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
			5*time.Second,
			5*time.Second,
		),
	})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, vdController)
	if err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualDisk{}).
		WithValidator(NewValidator(mgr.GetClient(), scService, disk)).
		Complete(); err != nil {
		return nil, err
	}

	vdcolelctor.SetupCollector(mgr.GetCache(), metrics.Registry, log)

	log.Info("Initialized VirtualDisk controller", "image", importerImage)

	return vdController, nil
}
