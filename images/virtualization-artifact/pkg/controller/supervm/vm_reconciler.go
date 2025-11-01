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

package supervm

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supervm/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error)
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

type Reconciler struct {
	client   client.Client
	handlers []Handler
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &v1alpha2.VirtualMachine{}, &handler.TypedEnqueueRequestForObject[*v1alpha2.VirtualMachine]{})); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewKVVMWatcher(),
		watcher.NewKVVMIWatcher(),
		watcher.NewPodWatcher(),
		watcher.NewVirtualImageWatcher(mgr.GetClient()),
		watcher.NewClusterVirtualImageWatcher(mgr.GetClient()),
		watcher.NewVirtualDiskWatcher(mgr.GetClient()),
		watcher.NewVMIPWatcher(),
		watcher.NewVirtualMachineClassWatcher(),
		watcher.NewVirtualMachineSnapshotWatcher(),
		watcher.NewVMOPWatcher(),
		watcher.NewVMMACWatcher(),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	vm := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vm.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vm.IsEmpty() {
		log.Info("Reconcile observe an absent VirtualMachine: it may be deleted")
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, vm.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		return vm.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus {
	return obj.Status
}
