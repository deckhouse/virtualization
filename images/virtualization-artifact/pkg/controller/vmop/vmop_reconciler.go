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

package vmop

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, s state.VMOperationState) (reconcile.Result, error)
	Name() string
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

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineOperation{}), &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("error setting watch on VMOP: %w", err)
	}
	// Subscribe on VirtualMachines.
	if err = ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, vm client.Object) []reconcile.Request {
			c := mgr.GetClient()
			vmops := &virtv2.VirtualMachineOperationList{}
			if err := c.List(ctx, vmops, client.InNamespace(vm.GetNamespace())); err != nil {
				return nil
			}
			var requests []reconcile.Request
			for _, vmop := range vmops.Items {
				if vmop.Spec.VirtualMachine == vm.GetName() && vmop.Status.Phase == virtv2.VMOPPhaseInProgress {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vmop.GetNamespace(),
							Name:      vmop.GetName(),
						},
					})
					break
				}
			}
			return requests
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVM := e.ObjectOld.(*virtv2.VirtualMachine)
				newVM := e.ObjectNew.(*virtv2.VirtualMachine)
				return oldVM.Status.Phase != newVM.Status.Phase || newVM.Status.MigrationState != nil
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}

	// Subscribe on VirtualMachineInstanceMigration.
	if err = ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstanceMigration{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			migration, ok := obj.(*virtv1.VirtualMachineInstanceMigration)
			if !ok {
				return nil
			}
			c := mgr.GetClient()

			vmops := &virtv2.VirtualMachineOperationList{}
			if err := c.List(ctx, vmops, client.InNamespace(obj.GetNamespace())); err != nil {
				return nil
			}

			var requests []reconcile.Request
			for _, vmop := range vmops.Items {
				if vmop.Spec.VirtualMachine == migration.Spec.VMIName {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: vmop.GetNamespace(),
							Name:      vmop.GetName(),
						},
					})
				}
			}

			return requests
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldMigration := e.ObjectOld.(*virtv1.VirtualMachineInstanceMigration)
				newMigration := e.ObjectNew.(*virtv1.VirtualMachineInstanceMigration)
				return oldMigration.Status.Phase != newMigration.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	vmop := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmop.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmop.IsEmpty() {
		return reconcile.Result{}, nil
	}

	log.Info("Start reconcile VMOP", "namespacedName", req.String())

	s := state.New(r.client, vmop)
	var handlerErrs []error

	var result reconcile.Result
	for _, h := range r.handlers {
		log.Debug("Run handler", "name", h.Name())
		res, err := h.Handle(ctx, s)
		if err != nil {
			log.Error("Failed to handle VirtualMachineOperation", "err", err, "handler", h.Name())
			handlerErrs = append(handlerErrs, err)
		}

		result = service.MergeResults(result, res)
	}

	err = vmop.Update(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = errors.Join(handlerErrs...)
	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}

func (r *Reconciler) factory() *virtv2.VirtualMachineOperation {
	return &virtv2.VirtualMachineOperation{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineOperation) virtv2.VirtualMachineOperationStatus {
	return obj.Status
}
