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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, s state.VMIPState) (reconcile.Result, error)
	Name() string
}

type Reconciler struct {
	handlers []Handler
	client   client.Client
}

func NewReconciler(client client.Client, handlers ...Handler) (*Reconciler, error) {
	return &Reconciler{
		client:   client,
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
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
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

	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddress{}), &handler.EnqueueRequestForObject{})
}

func (r *Reconciler) enqueueRequestsFromVMs(ctx context.Context, obj client.Object) []reconcile.Request {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return nil
	}

	var requests []reconcile.Request
	if vm.Spec.VirtualMachineIPAddress == "" {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      vm.Name,
			Namespace: vm.Namespace,
		}})
	} else {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: vm.Namespace,
			Name:      vm.Spec.VirtualMachineIPAddress,
		}})
	}

	vmipList := &virtv2.VirtualMachineIPAddressList{}
	err := r.client.List(ctx, vmipList, client.InNamespace(vm.Namespace),
		&client.MatchingFields{
			indexer.IndexFieldVMIPByVM: vm.Name,
		})
	if err != nil {
		return nil
	}

	for _, vmip := range vmipList.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: vmip.Namespace,
			Name:      vmip.Name,
		}})
	}

	return requests
}

func (r *Reconciler) enqueueRequestsFromLeases(_ context.Context, obj client.Object) []reconcile.Request {
	lease, ok := obj.(*virtv2.VirtualMachineIPAddressLease)
	if !ok {
		return nil
	}

	if lease.Spec.VirtualMachineIPAddressRef == nil {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: lease.Spec.VirtualMachineIPAddressRef.Namespace,
				Name:      lease.Spec.VirtualMachineIPAddressRef.Name,
			},
		},
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	vmip := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmip.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmip.IsEmpty() {
		return reconcile.Result{}, nil
	}

	log.Debug("Start reconcile VMIP")

	s := state.New(r.client, vmip.Changed())
	var handlerErrs []error

	var result reconcile.Result
	for _, h := range r.handlers {
		log.Debug("Run handler", logger.SlogHandler(h.Name()))

		var res reconcile.Result
		res, err = h.Handle(ctx, s)
		if err != nil {
			log.Error("Failed to handle VirtualMachineIP", logger.SlogErr(err), logger.SlogHandler(h.Name()))
			handlerErrs = append(handlerErrs, err)
		}

		result = service.MergeResults(result, res)
	}

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

func (r *Reconciler) factory() *virtv2.VirtualMachineIPAddress {
	return &virtv2.VirtualMachineIPAddress{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineIPAddress) virtv2.VirtualMachineIPAddressStatus {
	return obj.Status
}
