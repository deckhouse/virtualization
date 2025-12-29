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

package cvi

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/postponehandler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	cvicollector "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/cvi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "cvi-controller"

	PodVerbose    = "3"
	PodPullPolicy = string(corev1.PullIfNotPresent)
)

type Condition interface {
	Handle(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) error
}

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	importerImage string,
	uploaderImage string,
	requirements corev1.ResourceRequirements,
	dvcrSettings *dvcr.Settings,
	ns string,
) (controller.Controller, error) {
	stat := service.NewStatService(log)
	protection := service.NewProtectionService(mgr.GetClient(), v1alpha2.FinalizerCVIProtection)
	importer := service.NewImporterService(dvcrSettings, mgr.GetClient(), importerImage, requirements, PodPullPolicy, PodVerbose, ControllerName, protection)
	uploader := service.NewUploaderService(dvcrSettings, mgr.GetClient(), uploaderImage, requirements, PodPullPolicy, PodVerbose, ControllerName, protection)
	disk := service.NewDiskService(mgr.GetClient(), dvcrSettings, protection, ControllerName)
	dvcrService := service.NewDVCRService(mgr.GetClient())
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)

	sources := source.NewSources()
	sources.Set(v1alpha2.DataSourceTypeHTTP, source.NewHTTPDataSource(recorder, stat, importer, dvcrSettings, ns))
	sources.Set(v1alpha2.DataSourceTypeContainerImage, source.NewRegistryDataSource(recorder, stat, importer, dvcrSettings, mgr.GetClient(), ns))
	sources.Set(v1alpha2.DataSourceTypeObjectRef, source.NewObjectRefDataSource(recorder, stat, importer, disk, dvcrSettings, mgr.GetClient(), ns))
	sources.Set(v1alpha2.DataSourceTypeUpload, source.NewUploadDataSource(recorder, stat, uploader, dvcrSettings, ns))

	reconciler := NewReconciler(
		mgr.GetClient(),
		postponehandler.New[*v1alpha2.ClusterVirtualImage](dvcrService, recorder),
		internal.NewDatasourceReadyHandler(sources),
		internal.NewLifeCycleHandler(sources, mgr.GetClient()),
		internal.NewImagePresenceHandler(dvcr.NewImageChecker(mgr.GetClient(), dvcrSettings)),
		internal.NewDeletionHandler(sources),
		internal.NewAttacheeHandler(mgr.GetClient()),
	)

	cviController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, cviController)
	if err != nil {
		return nil, err
	}

	if dvcrSettings.ImageMonitorSchedule != "" {
		lister := gc.NewObjectLister(func(ctx context.Context, now time.Time) ([]client.Object, error) {
			cviList := &v1alpha2.ClusterVirtualImageList{}
			fieldSelector := fields.OneTermEqualSelector(indexer.IndexFieldCVIByPhase, indexer.ReadyDVCRImage)
			if err := mgr.GetClient().List(ctx, cviList, &client.ListOptions{FieldSelector: fieldSelector}); err != nil {
				return nil, err
			}

			objs := make([]client.Object, 0, len(cviList.Items))
			for i := range cviList.Items {
				objs = append(objs, &cviList.Items[i])
			}
			return objs, nil
		})

		cronSource, err := gc.NewCronSource(dvcrSettings.ImageMonitorSchedule, lister, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create cron source for image monitoring: %w", err)
		}

		if err := cviController.Watch(cronSource); err != nil {
			return nil, fmt.Errorf("failed to setup periodic image check: %w", err)
		}
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.ClusterVirtualImage{}).
		WithValidator(NewValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	cvicollector.SetupCollector(mgr.GetCache(), metrics.Registry, log)

	log.Info("Initialized ClusterVirtualImage controller", "image", importerImage)

	return cviController, nil
}
