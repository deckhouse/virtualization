package two_phase_reconciler

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	ReconcilerCore ReconcilerCore

	Client   client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
	Log      logr.Logger
}

func NewReconciler(reconcilerCore ReconcilerCore, client client.Client, recorder record.EventRecorder, scheme *runtime.Scheme, log logr.Logger) *Reconciler {
	return &Reconciler{
		ReconcilerCore: reconcilerCore,
		Client:         client,
		Recorder:       recorder,
		Scheme:         scheme,
		Log:            log,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	state := r.ReconcilerCore.NewReconcilerState(r)

	if err := state.Reload(ctx, req, r.Log, r.Client); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to reload reconciler state: %w", err)
	}

	var res *reconcile.Result
	var resErr error

	if state.ShouldReconcile() {
		r.Log.Info("sync phase begin")
		syncErr := r.sync(ctx, req, state)
		resErr = syncErr
		res = state.GetReconcilerResult()
		if state.ShouldApplyUpdateStatus() {
			if err := state.ApplyUpdateStatus(ctx, r.Log); err != nil {
				return reconcile.Result{}, fmt.Errorf("apply update status failed: %w", err)
			}
		}
		r.Log.Info("sync phase end")
	}

	if err := state.Reload(ctx, req, r.Log, r.Client); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to reload reconciler state: %w", err)
	}

	if state.ShouldReconcile() {
		r.Log.Info("update status phase begin")
		updateStatusErr := r.updateStatus(ctx, req, state)
		if res == nil {
			res = state.GetReconcilerResult()
		}
		if resErr == nil {
			resErr = updateStatusErr
		}
		r.Log.Info("update status phase end")
	}

	if res == nil {
		res = &reconcile.Result{}
	}
	return *res, resErr
}

func (r *Reconciler) sync(ctx context.Context, req reconcile.Request, state ReconcilerState) error {
	if err := r.ReconcilerCore.Sync(ctx, req, state, r); err != nil {
		return err
	}
	if err := state.ApplySync(ctx, r.Log); err != nil {
		return fmt.Errorf("unable to apply sync changes: %w", err)
	}
	return nil
}

func (r *Reconciler) updateStatus(ctx context.Context, req reconcile.Request, state ReconcilerState) error {
	if err := r.ReconcilerCore.UpdateStatus(ctx, req, state, r); err != nil {
		return fmt.Errorf("update status phase failed: %w", err)
	}
	if err := state.ApplyUpdateStatus(ctx, r.Log); err != nil {
		return fmt.Errorf("apply update status failed: %w", err)
	}
	return nil
}
