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

package dvcrmaintenance

import (
	"context"
	"fmt"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-maintenance/internal"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	ControllerName = "dvcr-maintenance-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	dvcrSettings *dvcr.Settings,
) (controller.Controller, error) {
	// init services
	reconciler := NewReconciler(
		mgr.GetClient(),
		internal.NewLifeCycleHandler(mgr.GetClient()),
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

	// Not an elegant solution, but it is easier to add cron watch here, than in internal/watcher package.
	cronSource, err := gc.NewCronSource(dvcrSettings.GCSchedule, gc.NewSingleObjectLister("__cron_injected__", "run_auto_cleanup"), log)
	if err != nil {
		return nil, fmt.Errorf("setup DVCR cleanup cron source: %w", err)
	}
	err = dvcrController.Watch(cronSource)
	if err != nil {
		return nil, fmt.Errorf("faield to setup cron watcher: %w", err)
	}

	log.Info("Initialized DVCR maintenance controller")

	return dvcrController, nil
}
