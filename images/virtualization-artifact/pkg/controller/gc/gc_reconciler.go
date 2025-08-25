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

package gc

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	dlog "github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

type Reconciler struct {
	client      client.Client
	watchSource source.Source
	mgr         ReconcileGCManager
}

func NewReconciler(client client.Client, watchSource source.Source, mgr ReconcileGCManager) Reconciler {
	return Reconciler{
		client:      client,
		watchSource: watchSource,
		mgr:         mgr,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)
	obj := r.mgr.New()
	err := r.client.Get(ctx, request.NamespacedName, obj)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		return reconcile.Result{}, nil
	}

	if r.mgr.ShouldBeDeleted(obj) {
		log.Info("deleting object")
		return reconcile.Result{}, r.client.Delete(ctx, obj)
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) SetupWithManager(controllerName string, mgr ctrl.Manager, log *dlog.Logger) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(r.mgr.New(), builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(ue event.UpdateEvent) bool {
				return false
			},
			DeleteFunc: func(de event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(ge event.GenericEvent) bool {
				return false
			},
		})).
		WithOptions(controller.Options{
			RecoverPanic:     ptr.To(true),
			LogConstructor:   logger.NewConstructor(log),
			CacheSyncTimeout: 10 * time.Minute,
		}).
		WatchesRawSource(r.watchSource).
		Complete(r)
}
