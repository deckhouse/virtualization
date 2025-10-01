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

package vi

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

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/controller/watchers"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error)
	Name() string
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
	vi := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vi.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vi.IsEmpty() {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, vi.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vi.Changed().Status.ObservedGeneration = vi.Changed().Generation

		return vi.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{},
			&handler.TypedEnqueueRequestForObject[*virtv2.VirtualImage]{},
			predicate.TypedFuncs[*virtv2.VirtualImage]{
				UpdateFunc: func(e event.TypedUpdateEvent[*virtv2.VirtualImage]) bool {
					return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualImage: %w", err)
	}

	viFromVIEnqueuer := watchers.NewVirtualImageRequestEnqueuer(mgr.GetClient(), &virtv2.VirtualImage{}, virtv2.VirtualImageObjectRefKindVirtualImage)
	viWatcher := watchers.NewObjectRefWatcher(watchers.NewVirtualImageFilter(), viFromVIEnqueuer)
	if err := viWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on VIs: %w", err)
	}

	viFromCVIEnqueuer := watchers.NewVirtualImageRequestEnqueuer(mgr.GetClient(), &virtv2.ClusterVirtualImage{}, virtv2.VirtualImageObjectRefKindClusterVirtualImage)
	cviWatcher := watchers.NewObjectRefWatcher(watchers.NewClusterVirtualImageFilter(), viFromCVIEnqueuer)
	if err := cviWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on CVIs: %w", err)
	}

	mgrClient := mgr.GetClient()
	for _, w := range []Watcher{
		watcher.NewPodWatcher(mgrClient),
		watcher.NewStorageClassWatcher(mgrClient),
		watcher.NewVirtualMachineWatcher(mgrClient),
		watcher.NewVirtualDiskSnapshotWatcher(mgrClient),
		watcher.NewDataVolumeWatcher(),
		watcher.NewPersistentVolumeClaimWatcher(),
		watcher.NewVirtualDiskWatcher(mgrClient),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) factory() *virtv2.VirtualImage {
	return &virtv2.VirtualImage{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualImage) virtv2.VirtualImageStatus {
	return obj.Status
}
