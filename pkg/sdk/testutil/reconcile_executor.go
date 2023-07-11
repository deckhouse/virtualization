package testutil

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcileExecutor struct {
	Name types.NamespacedName

	LastReconcileResult *reconcile.Result
}

func NewReconcileExecutor(name types.NamespacedName) *ReconcileExecutor {
	return &ReconcileExecutor{
		Name: name,
	}
}

func (e *ReconcileExecutor) Execute(ctx context.Context, reconciler reconcile.Reconciler) error {
	for {
		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: e.Name})
		if err != nil {
			return err
		}
		e.LastReconcileResult = &res

		if !res.Requeue {
			return nil
		}
	}
}
