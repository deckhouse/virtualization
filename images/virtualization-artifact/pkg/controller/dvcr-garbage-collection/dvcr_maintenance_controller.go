/*
Copyright 2025 Flant JSC

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

package dvcrgarbagecollection

import (
	"context"
	"fmt"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/internal"
	internalservice "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/internal/service"
	dvcrtypes "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/types"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	ControllerName = "dvcr-garbage-collection-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	dvcrSettings *dvcr.Settings,
) (controller.Controller, error) {
	// TODO: Remove this "if" to re-enable default schedule for cleanup.
	if dvcrSettings.GCSchedule == "" {
		log.Info("Initializing DVCR maintenance controller is disabled: set spec.settings.dvcr.gc.schedule in ModuleConfig to run garbage collector periodically.")
		return nil, nil
	}
	// init services
	dvcrService := service.NewDVCRService(mgr.GetClient())
	provisioningLister := internalservice.NewProvisioningLister(mgr.GetClient())

	reconciler := NewReconciler(
		mgr.GetClient(),
		internal.NewLifeCycleHandler(mgr.GetClient(), dvcrService, provisioningLister),
	)

	dvcrController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
	})
	if err != nil {
		return nil, err
	}

	if err = reconciler.SetupController(ctx, mgr, dvcrController); err != nil {
		return nil, err
	}

	// Not an elegant solution, but it is easier to add cron watches here, than in internal/watcher package.
	// Cron source to initiate garbage collection from the user specified schedule.
	cronSourceGC, err := gc.NewCronSource(dvcrSettings.GCSchedule, gc.NewSingleObjectLister(dvcrtypes.CronSourceNamespace, dvcrtypes.CronSourceRunGC), log)
	if err != nil {
		return nil, fmt.Errorf("setup DVCR cleanup cron source: %w", err)
	}
	err = dvcrController.Watch(cronSourceGC)
	if err != nil {
		return nil, fmt.Errorf("failed to setup dvcr-cleanup cron watcher: %w", err)
	}

	log.Info("Initialized DVCR garbage collection controller")

	return dvcrController, nil
}
