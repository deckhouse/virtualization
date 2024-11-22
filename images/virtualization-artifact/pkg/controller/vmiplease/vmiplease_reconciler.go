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

package vmiplease

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, s state.VMIPLeaseState) (reconcile.Result, error)
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
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddress{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromVMIP),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return false },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on vmip: %w", err)
	}

	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressLease{}), &handler.EnqueueRequestForObject{})
}

func (r *Reconciler) enqueueRequestsFromVMIP(_ context.Context, obj client.Object) []reconcile.Request {
	vmip, ok := obj.(*virtv2.VirtualMachineIPAddress)
	if !ok {
		return nil
	}

	if vmip.Status.Address == "" {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name: ip.IpToLeaseName(vmip.Status.Address),
			},
		},
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	lease := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	var err error
	err = lease.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if lease.IsEmpty() {
		return reconcile.Result{}, nil
	}

	log.Debug("Start reconcile VMIPLease")

	s := state.New(r.client, lease.Changed())
	var handlerErrs []error

	var result reconcile.Result
	for _, h := range r.handlers {
		log.Debug("Run handler", logger.SlogHandler(h.Name()))

		var res reconcile.Result
		res, err = h.Handle(ctx, s)
		if err != nil {
			log.Error("Failed to handle VMIPLease", "err", err, logger.SlogHandler(h.Name()))
			handlerErrs = append(handlerErrs, err)
		}

		result = service.MergeResults(result, res)
	}

	if s.ShouldDeletion() {
		err = r.client.Delete(ctx, lease.Changed())
	} else {
		if !reflect.DeepEqual(lease.Current().Spec, lease.Changed().Spec) {
			leaseStatus := lease.Changed().Status.DeepCopy()
			err = r.client.Update(ctx, lease.Changed())
			if err != nil {
				log.Error("Failed to update VirtualMachineIPAddressLease", "err", err)
				handlerErrs = append(handlerErrs, err)
			}
			lease.Changed().Status = *leaseStatus
		}

		err = lease.Update(ctx)
	}

	if err != nil {
		handlerErrs = append(handlerErrs, err)
	}

	err = errors.Join(handlerErrs...)
	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
}

func (r *Reconciler) factory() *virtv2.VirtualMachineIPAddressLease {
	return &virtv2.VirtualMachineIPAddressLease{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineIPAddressLease) virtv2.VirtualMachineIPAddressLeaseStatus {
	return obj.Status
}
