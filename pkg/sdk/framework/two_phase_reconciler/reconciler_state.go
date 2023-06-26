package two_phase_reconciler

import (
	"context"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcilerState interface {
	ReconcilerStateApplier

	SetReconcilerResult(result *reconcile.Result)
	GetReconcilerResult() *reconcile.Result
	Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error
}

type ReconcilerStateApplier interface {
	ApplySync(ctx context.Context, log logr.Logger) error
	ApplyUpdateStatus(ctx context.Context, log logr.Logger) error
	ShouldApplyUpdateStatus() bool
	ShouldReconcile() bool
}
