/*
Copyright 2025 Flant JSC

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

package reconciler

import (
	"context"
	"errors"
	"reflect"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

var ErrStopHandlerChain = errors.New("stop handler chain execution")

type ResourceUpdater func(ctx context.Context) error

type HandlerExecutor[H any] func(ctx context.Context, h H) (reconcile.Result, error)

type BaseReconciler[H any] struct {
	handlers []H
	update   ResourceUpdater
	execute  HandlerExecutor[H]
}

func NewBaseReconciler[H any](handlers []H) *BaseReconciler[H] {
	return &BaseReconciler[H]{
		handlers: handlers,
	}
}

func (r *BaseReconciler[H]) SetResourceUpdater(update ResourceUpdater) {
	r.update = update
}

func (r *BaseReconciler[H]) SetHandlerExecutor(execute HandlerExecutor[H]) {
	r.execute = execute
}

func (r *BaseReconciler[H]) Reconcile(ctx context.Context) (reconcile.Result, error) {
	if r.update == nil {
		return reconcile.Result{}, errors.New("update func is omitted: cannot reconcile")
	}

	if r.execute == nil {
		return reconcile.Result{}, errors.New("execute func is omitted: cannot reconcile")
	}

	logger.FromContext(ctx).Debug("Start reconciliation")

	var result reconcile.Result
	var errs error

handlersLoop:
	for _, h := range r.handlers {
		log := logger.FromContext(ctx).With(logger.SlogHandler(reflect.TypeOf(h).Elem().Name()))

		res, err := r.execute(ctx, h)
		switch {
		case err == nil: // OK.
		case errors.Is(err, ErrStopHandlerChain):
			log.Debug("Handler chain execution stopped")
			result = MergeResults(result, res)
			break handlersLoop
		case k8serrors.IsConflict(err):
			log.Debug("Conflict occurred during handler execution", logger.SlogErr(err))
			result.RequeueAfter = 100 * time.Microsecond
		default:
			log.Error("The handler failed with an error", logger.SlogErr(err))
			errs = errors.Join(errs, err)
		}

		result = MergeResults(result, res)
	}

	err := r.update(ctx)
	switch {
	case err == nil: // OK.
	case k8serrors.IsConflict(err):
		logger.FromContext(ctx).Debug("Conflict occurred during resource update", logger.SlogErr(err))
		result.RequeueAfter = 100 * time.Microsecond
	default:
		logger.FromContext(ctx).Error("Failed to update resource", logger.SlogErr(err))
		errs = errors.Join(errs, err)
	}

	if errs != nil {
		logger.FromContext(ctx).Error("Error occurred during reconciliation", logger.SlogErr(errs))
		return reconcile.Result{}, errs
	}

	//nolint:staticcheck // logging
	logger.FromContext(ctx).Debug("Reconciliation was successfully completed", "requeue", result.Requeue, "after", result.RequeueAfter)

	return result, nil
}
