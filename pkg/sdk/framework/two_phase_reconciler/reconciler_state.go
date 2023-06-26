package two_phase_reconciler

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcilerStateFactory[T ReconcilerState] func(name types.NamespacedName, log logr.Logger, client client.Client) T

type ReconcilerState interface {
	ReconcilerStateApplier

	SetReconcilerResult(result *reconcile.Result)
	GetReconcilerResult() *reconcile.Result

	Reload(ctx context.Context, req reconcile.Request, log logr.Logger, client client.Client) error
	ShouldReconcile() bool
	ShouldApplyUpdateStatus() bool
}

type ReconcilerStateApplier interface {
	ApplySync(ctx context.Context, log logr.Logger) error
	ApplyUpdateStatus(ctx context.Context, log logr.Logger) error
}
