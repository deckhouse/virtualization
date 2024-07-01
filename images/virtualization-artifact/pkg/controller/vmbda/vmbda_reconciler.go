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

package vmbda

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/watcher"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error)
}

type Watcher interface {
	Run(mgr manager.Manager, ctr controller.Controller) error
}

type Reconciler struct {
	handlers []Handler
	client   client.Client
	logger   *slog.Logger
}

func NewReconciler(client client.Client, logger *slog.Logger, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		logger:   logger,
		handlers: handlers,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vmbda := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmbda.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmbda.IsEmpty() {
		return reconcile.Result{}, nil
	}

	var result reconcile.Result
	var handlerErrs []error

	for _, h := range r.handlers {
		var res reconcile.Result
		res, err = h.Handle(ctx, vmbda.Changed())
		if err != nil {
			r.logger.Error("Failed to handle vmbda", "err", err)
			handlerErrs = append(handlerErrs, err)
		}

		result = service.MergeResults(result, res)
	}

	vmbda.Changed().Status.ObservedGeneration = vmbda.Changed().Generation

	err = vmbda.Update(ctx)
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
		watcher.NewVirtualMachineBlockDeviceAttachmentWatcher(r.logger, mgr.GetClient()),
		watcher.NewVirtualMachineWatcher(r.logger, mgr.GetClient()),
		watcher.NewVirtualDiskWatcher(r.logger, mgr.GetClient()),
		watcher.NewKVVMIWatcher(r.logger, mgr.GetClient()),
	} {
		err := w.Run(mgr, ctr)
		if err != nil {
			return fmt.Errorf("faield to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) factory() *virtv2.VirtualMachineBlockDeviceAttachment {
	return &virtv2.VirtualMachineBlockDeviceAttachment{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineBlockDeviceAttachment) virtv2.VirtualMachineBlockDeviceAttachmentStatus {
	return obj.Status
}
