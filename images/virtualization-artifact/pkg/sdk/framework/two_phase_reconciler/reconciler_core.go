/*
Copyright 2024 Flant JSC

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
	r.ReconcilerOptions.Log.V(3).Info(fmt.Sprintf("Start Reconcile for %s", req.String()), "obj", req.String())
	state := r.StateFactory(req.NamespacedName, r.Log, r.Client, r.Cache)

	if err := state.Reload(ctx, req, r.Log, r.Client); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to reload reconciler state: %w", err)
	}

	var res *reconcile.Result
	var resErr error

	if state.ShouldReconcile(r.Log) {
		syncErr := r.sync(ctx, req, state)
		resErr = syncErr
		res = state.GetReconcilerResult()
		if res != nil {
			r.Log.V(3).Info(fmt.Sprintf("Reconcile result after Sync: %+v", res), "obj", req.String())
		}
	} else {
		r.Log.V(3).Info("No Sync required", "obj", req.String())
	}

	if state.ShouldReconcile(r.Log) {
		updateStatusErr := r.updateStatus(ctx, req, state)
		if res == nil {
			res = state.GetReconcilerResult()
		}
		if resErr == nil {
			resErr = updateStatusErr
		}
	} else {
		r.Log.V(3).Info("No UpdateStatus after Sync", "obj", req.String())
	}

	if res == nil {
		res = &reconcile.Result{}
	}
	r.Log.V(3).Info(fmt.Sprintf("Reconcile result after UpdateStatus: %+v", res), "obj", req.String())

	return *res, resErr
}

func (r *ReconcilerCore[T]) sync(ctx context.Context, req reconcile.Request, state T) error {
	r.ReconcilerOptions.Log.V(3).Info("Call Sync", "obj", req.String())
	if err := r.Reconciler.Sync(ctx, req, state, r.ReconcilerOptions); err != nil {
		return err
	}
	r.ReconcilerOptions.Log.V(3).Info("Call ApplySync", "obj", req.String())
	if err := state.ApplySync(ctx, r.Log); err != nil {
		return fmt.Errorf("unable to apply sync changes: %w", err)
	}
	return nil
}

func (r *ReconcilerCore[T]) updateStatus(ctx context.Context, req reconcile.Request, state T) error {
	r.ReconcilerOptions.Log.V(3).Info("Call UpdateStatus", "obj", req.String())
	if err := r.Reconciler.UpdateStatus(ctx, req, state, r.ReconcilerOptions); err != nil {
		return fmt.Errorf("update status phase failed: %w", err)
	}
	r.ReconcilerOptions.Log.V(3).Info("Call ApplyUpdateStatus", "obj", req.String())
	if err := state.ApplyUpdateStatus(ctx, r.Log); err != nil {
		return fmt.Errorf("apply update status failed: %w", err)
	}
	return nil
}
