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

package cvi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error)
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
	cvi := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := cvi.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if cvi.IsEmpty() {
		return reconcile.Result{}, nil
	}

	var requeue bool

	slog.Info("Start")

	var handlerErrs []error

	for _, h := range r.handlers {
		slog.Info("Handle...")
		var res reconcile.Result
		res, err = h.Handle(ctx, cvi.Changed())
		if err != nil {
			slog.Error("Failed to handle cvi", "err", err)
			handlerErrs = append(handlerErrs, err)
		}

		// TODO: merger.
		requeue = requeue || res.Requeue
	}

	cvi.Changed().Status.ObservedGeneration = cvi.Changed().Generation

	slog.Info("Update")

	err = cvi.Update(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = errors.Join(handlerErrs...)
	if err != nil {
		return reconcile.Result{}, err
	}

	if requeue {
		slog.Info("Requeue")
		return reconcile.Result{
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	slog.Info("Done")

	return reconcile.Result{}, nil
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.ClusterVirtualImage{}),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
		},
	); err != nil {
		return err
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueClusterImagesAttachedToVM()),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return r.vmHasAttachedClusterImages(e.Object)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return r.vmHasAttachedClusterImages(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return r.vmHasAttachedClusterImages(e.ObjectOld) || r.vmHasAttachedClusterImages(e.ObjectNew)
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VMs: %w", err)
	}

	return nil
}

func (r *Reconciler) factory() *virtv2.ClusterVirtualImage {
	return &virtv2.ClusterVirtualImage{}
}

func (r *Reconciler) statusGetter(obj *virtv2.ClusterVirtualImage) virtv2.ClusterVirtualImageStatus {
	return obj.Status
}

func (r *Reconciler) enqueueClusterImagesAttachedToVM() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		vm, ok := obj.(*virtv2.VirtualMachine)
		if !ok {
			return nil
		}

		var requests []reconcile.Request

		for _, bda := range vm.Status.BlockDeviceRefs {
			if bda.Kind != virtv2.ClusterImageDevice {
				continue
			}

			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
				Name: bda.Name,
			}})
		}

		return requests
	}
}

func (r *Reconciler) vmHasAttachedClusterImages(obj client.Object) bool {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return false
	}

	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == virtv2.ClusterImageDevice {
			return true
		}
	}

	return false
}
