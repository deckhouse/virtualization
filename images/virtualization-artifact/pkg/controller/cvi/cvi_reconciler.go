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

package cvi

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error)
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
	cvi := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := cvi.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if cvi.IsEmpty() {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, cvi.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		cvi.Changed().Status.ObservedGeneration = cvi.Changed().Generation

		return cvi.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&virtv2.ClusterVirtualImage{},
			&handler.TypedEnqueueRequestForObject[*virtv2.ClusterVirtualImage]{},
			predicate.TypedFuncs[*virtv2.ClusterVirtualImage]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.ClusterVirtualImage]) bool {
					return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
				},
			},
		),
	); err != nil {
		return err
	}

	mgrClient := mgr.GetClient()
	for _, w := range []Watcher{
		watcher.NewVirtualMachineWatcher(),
		watcher.NewVirtualDiskWatcher(mgrClient),
		watcher.NewVirtualDiskSnapshotWatcher(mgrClient),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	for _, w := range []Watcher{
		watcher.NewClusterVirtualImageWatcher(mgr.GetClient()),
		watcher.NewVirtualImageWatcher(mgr.GetClient()),
		watcher.NewVirtualDiskWatcher(mgr.GetClient()),
		watcher.NewVirtualDiskSnapshotWatcher(mgr.GetClient()),
		watcher.NewVirtualMachineBlockDeviceAttachmentWatcher(mgr.GetClient()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("error setting watcher: %w", err)
		}
	}

	return nil
}

func (r *Reconciler) factory() *virtv2.ClusterVirtualImage {
	return &virtv2.ClusterVirtualImage{}
}

func (r *Reconciler) statusGetter(obj *virtv2.ClusterVirtualImage) virtv2.ClusterVirtualImageStatus {
	return obj.Status
}
