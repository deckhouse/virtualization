package two_phase_reconciler

import (
	"context"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcilerCore[T ReconcilerState] interface {
	SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error
	Sync(ctx context.Context, req reconcile.Request, state T, opts ReconcilerOptions) error
	UpdateStatus(ctx context.Context, req reconcile.Request, state T, opts ReconcilerOptions) error
	NewReconcilerState(opts ReconcilerOptions) T
}
