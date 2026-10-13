/*
Copyright 2026 Flant JSC

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

package mainippool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/mainippool/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	// crdRetryInterval paces the wait for the kinds of the sdn module to be registered.
	crdRetryInterval = time.Minute
	// poolResyncInterval is how often the pool is reconciled without an event.
	poolResyncInterval = 10 * time.Minute
	// poolTerminatingRetryInterval paces the wait for a deleted pool to go away
	// before it is recreated.
	poolTerminatingRetryInterval = 10 * time.Second
)

// Handler takes no object, unlike the handlers of a resource controller: the pool
// and the reference to it are cluster-wide singletons, not the object of the request.
type Handler interface {
	Handle(ctx context.Context) error
	Name() string
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Reconciler struct {
	handlers []Handler
	watchers []Watcher
}

func NewReconciler(handlers []Handler, watchers []Watcher) *Reconciler {
	return &Reconciler{handlers: handlers, watchers: watchers}
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range r.watchers {
		if err := w.Watch(mgr, ctr); err != nil {
			return fmt.Errorf("error setting watcher %T: %w", w, err)
		}
	}
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	for _, h := range r.handlers {
		if err := h.Handle(ctx); err != nil {
			if errors.Is(err, handler.ErrPoolTerminating) {
				logger.FromContext(ctx).Info("The address pool of the Main network is being deleted, waiting to recreate it")
				return reconcile.Result{RequeueAfter: poolTerminatingRetryInterval}, nil
			}
			if meta.IsNoMatchError(err) {
				// The kind is not registered: the sdn module is still starting or too old
				// to have it. Ask again instead of waiting for an event nothing can deliver.
				return reconcile.Result{RequeueAfter: crdRetryInterval}, nil
			}
			return reconcile.Result{}, fmt.Errorf("handler %s: %w", h.Name(), err)
		}
	}
	// The pool itself is not watched: a watch on a kind an older sdn does not
	// register fails the setup. Periodic requeue restores it after a hand edit.
	return reconcile.Result{RequeueAfter: poolResyncInterval}, nil
}
