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

package supervd

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
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supervd/internal"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/supervd/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
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
	importerImage string,
	uploaderImage string,
	requirements corev1.ResourceRequirements,
	dvcr *dvcr.Settings,
	storageClassSettings config.VirtualDiskStorageClassSettings,
	logFactory logger.Factory,
	log *log.Logger,
) (controller.Controller, error) {
	protection := service.NewProtectionService(mgr.GetClient(), v1alpha2.FinalizerVDProtection)
	disk := service.NewDiskService(mgr.GetClient(), dvcr, protection, ControllerName)
	scService := intsvc.NewVirtualDiskStorageClassService(service.NewBaseStorageClassService(mgr.GetClient()), storageClassSettings)

	reconciler := NewReconciler(mgr.GetClient(), internal.NewSemaphoreHandler(
		mgr,
		logFactory,
		importerImage,
		uploaderImage,
		requirements,
		dvcr,
		storageClassSettings,
	))

	vdController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
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

	log.Info("Initialized SuperVirtualDisk controller", "image", importerImage)

	return vdController, nil
}
