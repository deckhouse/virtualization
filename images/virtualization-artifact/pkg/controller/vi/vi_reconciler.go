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
	"errors"
	"fmt"
	"log/slog"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/watchers"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

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
	vi := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vi.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vi.IsEmpty() {
		return reconcile.Result{}, nil
	}

	var requeue bool

	slog.Info("Start reconcile VI", slog.String("namespacedName", req.String()))

	var handlerErrs []error

	for _, h := range r.handlers {
		slog.Info("Run handler", slog.String("name", h.Name()))
		var res reconcile.Result
		res, err = h.Handle(ctx, vi.Changed())
		if err != nil {
			slog.Error("Failed to handle vi", "err", err)
			handlerErrs = append(handlerErrs, err)
		}

		requeue = requeue || res.Requeue
	}

	vi.Changed().Status.ObservedGeneration = vi.Changed().Generation

	err = vi.Update(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = errors.Join(handlerErrs...)
	if err != nil {
		return reconcile.Result{}, err
	}

	if requeue {
		slog.Info("Requeue for VI", slog.String("namespacedName", req.String()))
		return reconcile.Result{
			RequeueAfter: 2 * time.Second,
		}, nil
	}

	slog.Info("Finished reconcile VI", slog.String("namespacedName", req.String()))
	return reconcile.Result{}, nil
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{}),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
		},
	); err != nil {
		return err
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

	return nil
}

func (r *Reconciler) factory() *virtv2.VirtualImage {
	return &virtv2.VirtualImage{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualImage) virtv2.VirtualImageStatus {
	return obj.Status
}
