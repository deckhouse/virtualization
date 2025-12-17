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

package vi

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/postponehandler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	vicollector "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "vi-controller"

	PodVerbose    = "3"
	PodPullPolicy = string(corev1.PullIfNotPresent)
)

type Condition interface {
	Handle(ctx context.Context, vi *v1alpha2.VirtualImage) error
}

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	importerImage string,
	uploaderImage string,
	bounderImage string,
	requirements corev1.ResourceRequirements,
	dvcrSettings *dvcr.Settings,
	storageClassSettings config.VirtualImageStorageClassSettings,
) (controller.Controller, error) {
	stat := service.NewStatService(log)
	protection := service.NewProtectionService(mgr.GetClient(), v1alpha2.FinalizerVIProtection)
	importer := service.NewImporterService(dvcrSettings, mgr.GetClient(), importerImage, requirements, PodPullPolicy, PodVerbose, ControllerName, protection)
	uploader := service.NewUploaderService(dvcrSettings, mgr.GetClient(), uploaderImage, requirements, PodPullPolicy, PodVerbose, ControllerName, protection)
	bounder := service.NewBounderPodService(dvcrSettings, mgr.GetClient(), bounderImage, requirements, PodPullPolicy, PodVerbose, ControllerName, protection)
	disk := service.NewDiskService(mgr.GetClient(), dvcrSettings, protection, ControllerName)
	scService := intsvc.NewVirtualImageStorageClassService(service.NewBaseStorageClassService(mgr.GetClient()), storageClassSettings)
	dvcrService := service.NewDVCRService(mgr.GetClient())
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)

	sources := source.NewSources()
	sources.Set(v1alpha2.DataSourceTypeHTTP, source.NewHTTPDataSource(recorder, stat, importer, dvcrSettings, disk))
	sources.Set(v1alpha2.DataSourceTypeContainerImage, source.NewRegistryDataSource(recorder, stat, importer, dvcrSettings, mgr.GetClient(), disk))
	sources.Set(v1alpha2.DataSourceTypeObjectRef, source.NewObjectRefDataSource(recorder, stat, importer, bounder, dvcrSettings, mgr.GetClient(), disk))
	sources.Set(v1alpha2.DataSourceTypeUpload, source.NewUploadDataSource(recorder, stat, uploader, dvcrSettings, disk))

	reconciler := NewReconciler(
		mgr.GetClient(),
		dvcr.DefaultImageMonitorSchedule,
		log,
		postponehandler.New[*v1alpha2.VirtualImage](dvcrService, recorder),
		internal.NewStorageClassReadyHandler(recorder, scService),
		internal.NewDatasourceReadyHandler(sources),
		internal.NewLifeCycleHandler(recorder, sources, mgr.GetClient()),
		internal.NewImagePresenceHandler(mgr.GetClient(), dvcrSettings),
		internal.NewDeletionHandler(sources),
		internal.NewAttacheeHandler(mgr.GetClient()),
	)

	viController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, viController)
	if err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualImage{}).
		WithValidator(NewValidator(log, mgr.GetClient(), scService)).
		Complete(); err != nil {
		return nil, err
	}

	vicollector.SetupCollector(mgr.GetCache(), metrics.Registry, log)

	log.Info("Initialized VirtualImage controller", "image", importerImage)

	return viController, nil
}
