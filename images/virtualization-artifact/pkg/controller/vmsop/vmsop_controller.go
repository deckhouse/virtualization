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

package vmsop

import (
	"context"
	"time"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsop/internal/operation"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	vmsopcollector "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vmsop"
)

const ControllerName = "vmsop-controller"

func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
) error {
	l := log.With(logger.SlogController(ControllerName))
	client := mgr.GetClient()
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)
	createOp := operation.NewCreateVirtualMachineOperation(client)
	reconciler := NewReconciler(client,
		handler.NewLifecycleHandler(client, createOp, recorder),
		handler.NewDeletionHandler(client),
	)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RateLimiter:      workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](time.Second, 32*time.Second),
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(l),
		CacheSyncTimeout: 10 * time.Minute,
	})
	if err != nil {
		return err
	}

	err = reconciler.SetupController(ctx, mgr, c)
	if err != nil {
		return err
	}

	vmsopcollector.SetupCollector(mgr.GetCache(), metrics.Registry, log)

	log.Info("Initialized VirtualMachineSnapshotOperation controller")
	return nil
}
