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
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

var ErrStopHandlerChain = errors.New("stop handler chain execution")

type Handler[T client.Object] interface {
	Handle(ctx context.Context, obj T) (reconcile.Result, error)
	Name() string
}

type Named interface {
	Name() string
}

type Finalizer interface {
	Finalize(ctx context.Context) error
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}
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
		var name string
		if named, ok := any(h).(Named); ok {
			name = named.Name()
		} else {
			name = reflect.TypeOf(h).Elem().Name()
		}
		log := logger.FromContext(ctx).With(logger.SlogHandler(name))

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
	case strings.Contains(err.Error(), "no new finalizers can be added if the object is being deleted"):
		logger.FromContext(ctx).Warn("Forbidden to add finalizers", logger.SlogErr(err))
		result.RequeueAfter = 1 * time.Second
	case k8serrors.IsNotFound(err) && strings.Contains(err.Error(), "namespaces"):
		// No need to return an explicit requeue or requeue with error if namespace is gone, e.g. in e2e tests.
		logger.FromContext(ctx).Warn("Namespace is gone while reconcile the object", logger.SlogErr(err))
	default:
		logger.FromContext(ctx).Error("Failed to update resource", logger.SlogErr(err))
		errs = errors.Join(errs, err)
	}

	if errs != nil {
		logger.FromContext(ctx).Error("Error occurred during reconciliation", logger.SlogErr(errs))
		return reconcile.Result{}, errs
	}

	for _, h := range r.handlers {
		if finalizer, ok := any(h).(Finalizer); ok {
			if err := finalizer.Finalize(ctx); err != nil {
				logger.FromContext(ctx).Error("Failed to finalize resource", logger.SlogErr(err))
				return reconcile.Result{}, err
			}
		}
	}

	//nolint:staticcheck // logging
	logger.FromContext(ctx).Debug("Reconciliation was successfully completed", "requeue", result.Requeue, "after", result.RequeueAfter)

	return result, nil
}
