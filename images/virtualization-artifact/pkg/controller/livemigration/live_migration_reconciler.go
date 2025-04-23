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
	"fmt"

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	internalservice "github.com/deckhouse/virtualization-controller/pkg/controller/livemigration/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/livemigration/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) (reconcile.Result, error)
	Name() string
}

type Reconciler struct {
	handlers []Handler
	client   client.Client
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		handlers: handlers,
		client:   client,
	}
}

// SetupController adds watchers.
func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range []Watcher{
		watcher.NewKVVMIWatcher(),
		watcher.NewKVVMIMWatcher(),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("error setting watcher: %w", err)
		}
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	kvvmi := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := kvvmi.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi.IsEmpty() {
		log.Info("Reconcile observe an absent VirtualMachineInstance: it may be deleted")
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, kvvmi.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		if internalservice.IsMigrationConfigurationChanged(kvvmi.Current(), kvvmi.Changed()) {
			// Directly update kvvmi and not use kvvmi.Update as kvvmi status is a regular field, not a subresource.
			log.Debug("About to update changed kvvmi",
				"changed.migration.configuration", internalservice.DumpKVVMIMigrationConfiguration(kvvmi.Changed()),
				"current.migration.configuration", internalservice.DumpKVVMIMigrationConfiguration(kvvmi.Current()),
			)
			if err := r.client.Update(ctx, kvvmi.Changed()); err != nil {
				return fmt.Errorf("error updating status subresource: %w", err)
			}
			return nil
		}

		log.Debug("Reconcile kvvmi without updating",
			"current.migration.configuration", internalservice.DumpKVVMIMigrationConfiguration(kvvmi.Current()),
		)
		return nil
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *virtv1.VirtualMachineInstance {
	return &virtv1.VirtualMachineInstance{}
}

func (r *Reconciler) statusGetter(obj *virtv1.VirtualMachineInstance) virtv1.VirtualMachineInstanceStatus {
	return obj.Status
}
