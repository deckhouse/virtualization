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

package vdsnapshot

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vdsnapshot/internal/watcher"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (reconcile.Result, error)
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Reconciler struct {
	handlers []Handler
	client   client.Client
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vdSnapshot := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vdSnapshot.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vdSnapshot.IsEmpty() {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, vdSnapshot.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vdSnapshot.Changed().Status.ObservedGeneration = vdSnapshot.Changed().Generation

		return vdSnapshot.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range []Watcher{
		watcher.NewVirtualDiskSnapshotWatcher(mgr.GetClient()),
		watcher.NewVirtualDiskWatcher(mgr.GetClient()),
		watcher.NewVolumeSnapshotWatcher(),
		watcher.NewVirtualMachineWatcher(mgr.GetClient()),
		watcher.NewKVVMIWatcher(mgr.GetClient()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) factory() *v1alpha2.VirtualDiskSnapshot {
	return &v1alpha2.VirtualDiskSnapshot{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.VirtualDiskSnapshot) v1alpha2.VirtualDiskSnapshotStatus {
	return obj.Status
}
