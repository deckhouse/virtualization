//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package vmpool

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/expectations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, pool *v1alpha2.VirtualMachinePool) (reconcile.Result, error)
	Name() string
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

func NewReconciler(client client.Client, exp *expectations.Expectations, handlers []Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		exp:      exp,
		handlers: handlers,
	}
}

type Reconciler struct {
	client   client.Client
	exp      *expectations.Expectations
	handlers []Handler
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range []Watcher{
		watcher.NewVirtualMachinePoolWatcher(),
		watcher.NewVirtualMachineWatcher(r.exp),
	} {
		if err := w.Watch(mgr, ctr); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	pool := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := pool.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if pool.IsEmpty() {
		log.Info("Reconcile observe an absent VirtualMachinePool: it may be deleted")
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, pool.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		return pool.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *v1alpha2.VirtualMachinePool {
	return &v1alpha2.VirtualMachinePool{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.VirtualMachinePool) v1alpha2.VirtualMachinePoolStatus {
	return obj.Status
}
