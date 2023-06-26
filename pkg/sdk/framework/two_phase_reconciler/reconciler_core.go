package two_phase_reconciler

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcilerCore interface {
	SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error
	Sync(ctx context.Context, req reconcile.Request, state ReconcilerState, reconciler *Reconciler) error
	UpdateStatus(ctx context.Context, req reconcile.Request, state ReconcilerState, reconciler *Reconciler) error
	NewReconcilerState(reconciler *Reconciler) ReconcilerState
}
