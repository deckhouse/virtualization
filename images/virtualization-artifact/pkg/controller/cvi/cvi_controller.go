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
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "cvi-controller"

	PodVerbose    = "3"
	PodPullPolicy = string(corev1.PullIfNotPresent)
)

type Condition interface {
	Handle(ctx context.Context, cvi *virtv2.ClusterVirtualImage) error
}

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *slog.Logger,
	importerImage string,
	uploaderImage string,
	dvcr *dvcr.Settings,
	ns string,
) (controller.Controller, error) {
	log = log.With(logger.SlogController(ControllerName))

	stat := service.NewStatService(log)
	protection := service.NewProtectionService(mgr.GetClient(), virtv2.FinalizerCVIProtection)
	importer := service.NewImporterService(dvcr, mgr.GetClient(), importerImage, PodPullPolicy, PodVerbose, ControllerName, protection)
	uploader := service.NewUploaderService(dvcr, mgr.GetClient(), uploaderImage, PodPullPolicy, PodVerbose, ControllerName, protection)

	sources := source.NewSources()
	sources.Set(virtv2.DataSourceTypeHTTP, source.NewHTTPDataSource(stat, importer, dvcr, ns))
	sources.Set(virtv2.DataSourceTypeContainerImage, source.NewRegistryDataSource(stat, importer, dvcr, mgr.GetClient(), ns))
	sources.Set(virtv2.DataSourceTypeObjectRef, source.NewObjectRefDataSource(stat, importer, dvcr, mgr.GetClient(), ns))
	sources.Set(virtv2.DataSourceTypeUpload, source.NewUploadDataSource(stat, uploader, dvcr, ns))

	reconciler := NewReconciler(
		mgr.GetClient(),
		internal.NewDatasourceReadyHandler(sources),
		internal.NewLifeCycleHandler(sources, mgr.GetClient()),
		internal.NewDeletionHandler(sources),
		internal.NewAttacheeHandler(mgr.GetClient()),
	)

	cviController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:     reconciler,
		RecoverPanic:   ptr.To(true),
		LogConstructor: logger.NewConstructor(log),
	})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, cviController)
	if err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&virtv2.ClusterVirtualImage{}).
		WithValidator(NewValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized ClusterVirtualImage controller", "image", importerImage)

	return cviController, nil
}
