/*
Copyright 2026 Flant JSC

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

// Package nodemaintenance keeps the dialogue about releasing a node on the node itself: the module
// asks for the permission to restart the machines that hold it, an administrator answers, and both
// annotations go away once the node is free. The permission is spent by the maintenance that used
// it, not revoked when the node reopens, so it may be given ahead of the works and still covers a
// single maintenance.
package nodemaintenance

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/nodemaintenance/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const ControllerName = "node-maintenance-controller"

func SetupController(ctx context.Context, mgr manager.Manager, log *log.Logger) error {
	client := mgr.GetClient()

	r := NewReconciler(client, handler.NewRestartApprovalHandler(client))

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

	log.Info("Initialized node maintenance controller")
	return nil
}
