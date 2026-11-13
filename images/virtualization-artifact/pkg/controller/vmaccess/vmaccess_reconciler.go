/*
Copyright 2026 Flant JSC

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

package vmaccess

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmaccess/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmaccess/internal/watcher"
)

type Handler interface {
	Handle(ctx context.Context, s *state.State) (reconcile.Result, error)
	Name() string
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

// Reconciler is keyed by virtual machine, not by claim: every decision about one virtual machine is made
// in one place, one at a time.
type Reconciler struct {
	client   client.Client
	handlers []Handler
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range []Watcher{
		watcher.NewClaimWatcher(),
		watcher.NewVirtualMachineWatcher(),
		watcher.NewKVVMWatcher(),
		watcher.NewKVVMIWatcher(),
		watcher.NewMigrationWatcher(),
	} {
		if err := w.Watch(mgr, ctr); err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	s := state.New(r.client, req.NamespacedName)
	if err := s.Load(ctx); err != nil {
		return reconcile.Result{}, err
	}

	// A filesystem request left by a snapshot taken before the module update needs a pass without any claim:
	// the lifecycle handler clears it.
	if len(s.Claims) == 0 && s.Lock == nil && !s.MalformedLock && !s.HasFilesystemRequest() {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		return s.PatchClaims(ctx)
	})

	return rec.Reconcile(ctx)
}
