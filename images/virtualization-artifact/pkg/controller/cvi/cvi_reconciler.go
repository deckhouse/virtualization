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

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal"
	"github.com/deckhouse/virtualization-controller/pkg/controller/cvi/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/watchers"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error)
}

type Reconciler struct {
	handlers             []Handler
	client               client.Client
	imageMonitorSchedule string
	log                  *log.Logger
}

func NewReconciler(client client.Client, imageMonitorSchedule string, log *log.Logger, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:               client,
		imageMonitorSchedule: imageMonitorSchedule,
		log:                  log,
		handlers:             handlers,
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
			&v1alpha2.ClusterVirtualImage{},
			&handler.TypedEnqueueRequestForObject[*v1alpha2.ClusterVirtualImage]{},
			predicate.TypedFuncs[*v1alpha2.ClusterVirtualImage]{
				UpdateFunc: func(e event.TypedUpdateEvent[*v1alpha2.ClusterVirtualImage]) bool {
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

	cviFromVIEnqueuer := watchers.NewClusterVirtualImageRequestEnqueuer(mgr.GetClient(), &v1alpha2.VirtualImage{}, v1alpha2.ClusterVirtualImageObjectRefKindVirtualImage)
	viWatcher := watchers.NewObjectRefWatcher(watchers.NewVirtualImageFilter(), cviFromVIEnqueuer)
	if err := viWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on VIs: %w", err)
	}

	cviFromCVIEnqueuer := watchers.NewClusterVirtualImageRequestEnqueuer(mgr.GetClient(), &v1alpha2.ClusterVirtualImage{}, v1alpha2.ClusterVirtualImageObjectRefKindClusterVirtualImage)
	cviWatcher := watchers.NewObjectRefWatcher(watchers.NewClusterVirtualImageFilter(), cviFromCVIEnqueuer)
	if err := cviWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on CVIs: %w", err)
	}

	if r.imageMonitorSchedule != "" {
		enqueuer := internal.NewPeriodicEnqueuer(mgr.GetClient())
		cronSource, err := gc.NewCronSource(r.imageMonitorSchedule, enqueuer, r.log.With("source", "image-monitor"))
		if err != nil {
			return fmt.Errorf("failed to create cron source for image monitoring: %w", err)
		}

		if err := ctr.Watch(cronSource); err != nil {
			return fmt.Errorf("failed to setup periodic image check: %w", err)
		}
	}

	return nil
}

func (r *Reconciler) factory() *v1alpha2.ClusterVirtualImage {
	return &v1alpha2.ClusterVirtualImage{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.ClusterVirtualImage) v1alpha2.ClusterVirtualImageStatus {
	return obj.Status
}
