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

package dvcrgarbagecollection

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/internal/watcher"
	dvcrtypes "github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-garbage-collection/types"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, req reconcile.Request, deploy *appsv1.Deployment) (reconcile.Result, error)
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
	deploy := reconciler.NewResource(dvcrtypes.DVCRDeploymentKey(), r.client, r.factory, r.statusGetter)
	err := deploy.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	// DVCR garbage collection is needless if Deploy/dvcr is absent.
	if deploy.IsEmpty() {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, req, deploy.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		return deploy.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range []Watcher{
		watcher.NewDVCRGarbageCollectionSecretWatcher(mgr.GetClient()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to setup watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) factory() *appsv1.Deployment {
	return &appsv1.Deployment{}
}

func (r *Reconciler) statusGetter(obj *appsv1.Deployment) appsv1.DeploymentStatus {
	return obj.Status
}
