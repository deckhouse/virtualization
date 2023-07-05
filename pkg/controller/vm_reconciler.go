package controller

import (
	"context"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type VMReconciler struct{}

func (r *VMReconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(&source.Kind{Type: &virtv2.VirtualMachine{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return err
	}

	return nil
}

func (r *VMReconciler) Sync(ctx context.Context, req reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.Info("VMReconciler.Sync")
	return nil
}

func (r *VMReconciler) UpdateStatus(ctx context.Context, req reconcile.Request, state *VMReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	opts.Log.Info("VMReconciler.UpdateStatus")
	return nil
}
