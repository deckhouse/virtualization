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

package vmip

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, s state.VMIPState) (reconcile.Result, error)
}

type Reconciler struct {
	handlers []Handler
	client   client.Client
	logger   logr.Logger
}

func NewReconciler(client client.Client, logger logr.Logger, handlers ...Handler) (*Reconciler, error) {

	return &Reconciler{
		client:   client,
		logger:   logger,
		handlers: handlers,
	}, nil
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressLease{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromLeases),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return false },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on leases: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromVMs),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on vms: %w", err)
	}

	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressClaim{}), &handler.EnqueueRequestForObject{})
}

func (r *Reconciler) enqueueRequestsFromVMs(_ context.Context, obj client.Object) []reconcile.Request {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return nil
	}

	if vm.Spec.VirtualMachineIPAddressClaim == "" {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: vm.Namespace,
					Name:      vm.Name,
				},
			},
		}
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: vm.Namespace,
				Name:      vm.Spec.VirtualMachineIPAddressClaim,
			},
		},
	}
}

func (r *Reconciler) enqueueRequestsFromLeases(_ context.Context, obj client.Object) []reconcile.Request {
	lease, ok := obj.(*virtv2.VirtualMachineIPAddressLease)
	if !ok {
		return nil
	}

	if lease.Spec.ClaimRef == nil {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: lease.Spec.ClaimRef.Namespace,
				Name:      lease.Spec.ClaimRef.Name,
			},
		},
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vmip := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmip.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmip.IsEmpty() {
		return reconcile.Result{}, nil
	}

	s := state.New(r.client, vmip)
	var handlerErrs []error

	var result reconcile.Result
	for _, h := range r.handlers {
		res, err := h.Handle(ctx, s)
		if err != nil {
			r.logger.Error(err, "Failed to handle VirtualMachineIP", "err", err, "handler", reflect.TypeOf(h).Elem().Name())
			handlerErrs = append(handlerErrs, err)
		}

		result = mergeResults(result, res)
	}

	vmip.Changed().Status.ObservedGeneration = vmip.Changed().Generation

	err = vmip.Update(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = errors.Join(handlerErrs...)
	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}

func (r *Reconciler) factory() *virtv2.VirtualMachineIPAddressClaim {
	return &virtv2.VirtualMachineIPAddressClaim{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineIPAddressClaim) virtv2.VirtualMachineIPAddressClaimStatus {
	return obj.Status
}

func mergeResults(results ...reconcile.Result) reconcile.Result {
	var result reconcile.Result
	for _, r := range results {
		if r.IsZero() {
			continue
		}
		if r.Requeue {
			return r
		}
		if result.IsZero() && r.RequeueAfter > 0 {
			result = r
			continue
		}
		if r.RequeueAfter > 0 && r.RequeueAfter < result.RequeueAfter {
			result.RequeueAfter = r.RequeueAfter
		}
	}
	return result
}
