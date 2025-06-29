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

package vdexport

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/internal/watcher"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, vdexport *v1alpha2.VirtualDataExport) (reconcile.Result, error)
	Name() string
}

type Reconciler struct {
	handlers          []Handler
	client            client.Client
	dataExportEnabled bool
}

func NewReconciler(client client.Client, dataExportEnabled bool, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:            client,
		dataExportEnabled: dataExportEnabled,
		handlers:          handlers,
	}
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	mgrClient := mgr.GetClient()
	watchers := []Watcher{
		watcher.NewVDExportWatcher(),
		watcher.NewDataExportWatcher(r.dataExportEnabled),
		watcher.NewVDWatcher(r.dataExportEnabled, mgrClient),
		watcher.NewCVIWatcher(mgrClient),
		watcher.NewVIWatcher(mgrClient),
		watcher.NewPodWatcher(),
	}
	for _, w := range watchers {
		if err := w.Watch(mgr, ctr); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vdexport := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vdexport.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vdexport.IsEmpty() {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, vdexport.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vdexport.Changed().Status.ObservedGeneration = vdexport.Changed().Generation

		return vdexport.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *v1alpha2.VirtualDataExport {
	return &v1alpha2.VirtualDataExport{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.VirtualDataExport) v1alpha2.VirtualDataExportStatus {
	return obj.Status
}
