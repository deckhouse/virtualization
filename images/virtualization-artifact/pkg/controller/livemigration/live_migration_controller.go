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

package livemigration

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/livemigration/internal"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	ControllerName = "live-migration-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	liveMigrationSettings config.LiveMigrationSettings,
) error {
	// recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)
	// mgrCache := mgr.GetCache()
	client := mgr.GetClient()
	vmopService := service.NewVMOperationService(client)

	handlers := []Handler{
		internal.NewDynamicSettingsHandler(client, liveMigrationSettings, vmopService),
	}
	r := NewReconciler(client, handlers...)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
	})
	if err != nil {
		return err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	log.Info("Initialized LiveMigrationSettings controller")
	return nil
}
