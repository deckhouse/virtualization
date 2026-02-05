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

package vmop

import (
	"context"
	"time"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/migration"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/snapshot"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	vmopcollector "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SubController interface {
	Name() string
	Watchers() []reconciler.Watcher
	Handlers() []reconciler.Handler[*v1alpha2.VirtualMachineOperation]
	ShouldReconcile(vmop *v1alpha2.VirtualMachineOperation) bool
}

const ControllerName = "vmop-controller"

func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
) error {
	client := mgr.GetClient()

	controllers := []SubController{
		powerstate.NewController(client, mgr),
		migration.NewController(client, mgr, featuregates.Default()),
		snapshot.NewController(client, mgr),
	}

	for _, ctr := range controllers {
		l := log.With("controller", ctr.Name())
		r := NewReconciler(client, ctr)

		c, err := controller.New(ctr.Name(), mgr, controller.Options{
			Reconciler:       r,
			RateLimiter:      workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](time.Second, 32*time.Second),
			RecoverPanic:     ptr.To(true),
			LogConstructor:   logger.NewConstructor(l),
			CacheSyncTimeout: 10 * time.Minute,
			UsePriorityQueue: ptr.To(true),
		})
		if err != nil {
			return err
		}

		err = r.SetupController(ctx, mgr, c)
		if err != nil {
			return err
		}
	}

	if err := builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineOperation{}).
		WithValidator(NewValidator(mgr.GetClient(), log)).
		Complete(); err != nil {
		return err
	}

	vmopcollector.SetupCollector(mgr.GetCache(), metrics.Registry, log)

	log.Info("Initialized VirtualMachineOperation controller")
	return nil
}
