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
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/state"
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
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vmop := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmop.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmop.IsEmpty() {
		return reconcile.Result{}, nil
	}

	s := state.New(r.client, vmop)

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vmop.Changed().Status.ObservedGeneration = vmop.Changed().Generation

		return vmop.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *virtv2.VirtualMachineOperation {
	return &virtv2.VirtualMachineOperation{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineOperation) virtv2.VirtualMachineOperationStatus {
	return obj.Status
}
