package two_phase_reconciler

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcilerOptions struct {
	Client   client.Client
	Cache    cache.Cache
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
	Log      logr.Logger
}

type ReconcilerCore[T ReconcilerState] struct {
	Reconciler   TwoPhaseReconciler[T]
	StateFactory ReconcilerStateFactory[T]
	ReconcilerOptions
}

func NewReconcilerCore[T ReconcilerState](reconciler TwoPhaseReconciler[T], stateFactory ReconcilerStateFactory[T], opts ReconcilerOptions) *ReconcilerCore[T] {
	return &ReconcilerCore[T]{
		Reconciler:        reconciler,
		StateFactory:      stateFactory,
		ReconcilerOptions: opts,
	}
}

func (r *ReconcilerCore[T]) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	state := r.StateFactory(req.NamespacedName, r.Log, r.Client, r.Cache)

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
			r.Log.Info("sync phase: after-sync status update")
			if err := state.ApplyUpdateStatus(ctx, r.Log); err != nil {
				return reconcile.Result{}, fmt.Errorf("apply update status failed: %w", err)
			}
		} else {
			r.Log.Info("sync phase: skip after-sync status update")
		}
		r.Log.Info("sync phase end")
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

func (r *ReconcilerCore[T]) sync(ctx context.Context, req reconcile.Request, state T) error {
	if err := r.Reconciler.Sync(ctx, req, state, r.ReconcilerOptions); err != nil {
		return err
	}
	if err := state.ApplySync(ctx, r.Log); err != nil {
		return fmt.Errorf("unable to apply sync changes: %w", err)
	}
	return nil
}

func (r *ReconcilerCore[T]) updateStatus(ctx context.Context, req reconcile.Request, state T) error {
	if err := r.Reconciler.UpdateStatus(ctx, req, state, r.ReconcilerOptions); err != nil {
		return fmt.Errorf("update status phase failed: %w", err)
	}
	if err := state.ApplyUpdateStatus(ctx, r.Log); err != nil {
		return fmt.Errorf("apply update status failed: %w", err)
	}
	return nil
}
