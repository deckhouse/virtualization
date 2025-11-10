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

package workloadupdater

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/workload-updater/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/workload-updater/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	ControllerName = "workload-updater-controller"
)

func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	firmwareImage,
	namespace,
	virtControllerName string,
) error {
	client := mgr.GetClient()

	handlers := []Handler{
		handler.NewFirmwareHandler(client, service.NewOneShotMigrationService(client, "firmware-update-"), firmwareImage, namespace, virtControllerName),
		handler.NewNodePlacementHandler(client, service.NewOneShotMigrationService(client, "nodeplacement-update-")),
	}
	r := NewReconciler(client, handlers)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	log.Info("Initialized workload-updater controller")
	return nil
}
