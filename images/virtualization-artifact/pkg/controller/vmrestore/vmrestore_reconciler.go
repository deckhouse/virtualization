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

package vmrestore

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmrestore/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, vmRestore *virtv2.VirtualMachineRestore) (reconcile.Result, error)
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Reconciler struct {
	handlers []Handler
	client   client.Client
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	vmRestore := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmRestore.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmRestore.IsEmpty() {
		return reconcile.Result{}, nil
	}

	var result reconcile.Result
	var handlerErrs []error

	for _, h := range r.handlers {
		var res reconcile.Result
		res, err = h.Handle(ctx, vmRestore.Changed())
		if err != nil {
			log.Error("Failed to handle vmRestore", logger.SlogErr(err), logger.SlogHandler(reflect.TypeOf(h).Elem().Name()))
			handlerErrs = append(handlerErrs, err)
		}

		result = service.MergeResults(result, res)
	}

	vmRestore.Changed().Status.ObservedGeneration = vmRestore.Changed().Generation

	err = vmRestore.Update(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = errors.Join(handlerErrs...)
	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range []Watcher{
		watcher.NewVirtualMachineRestoreWatcher(mgr.GetClient()),
		watcher.NewVirtualMachineSnapshotWatcher(mgr.GetClient()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("faield to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) factory() *virtv2.VirtualMachineRestore {
	return &virtv2.VirtualMachineRestore{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineRestore) virtv2.VirtualMachineRestoreStatus {
	return obj.Status
}
