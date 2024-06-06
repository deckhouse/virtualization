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

package vd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
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
	Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error)
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
	vd := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vd.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vd.IsEmpty() {
		return reconcile.Result{}, nil
	}

	var requeue bool

	slog.Info("Start")

	var handlerErrs []error

	for _, h := range r.handlers {
		var res reconcile.Result
		slog.Info("Handle... " + reflect.TypeOf(h).Elem().Name())
		res, err = h.Handle(ctx, vd.Changed())
		if err != nil {
			slog.Error("Failed to handle vd", "err", err, "handler", reflect.TypeOf(h).Elem().Name())
			handlerErrs = append(handlerErrs, err)
		}

		// TODO: merger.
		requeue = requeue || res.Requeue
	}

	vd.Changed().Status.ObservedGeneration = vd.Changed().Generation

	slog.Info("Update")

	err = vd.Update(ctx)
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
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{}),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &cdiv1.DataVolume{}),
		handler.EnqueueRequestForOwner(
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&virtv2.VirtualDisk{},
			handler.OnlyControllerOwner(),
		),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return false },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldDV, ok := e.ObjectOld.(*cdiv1.DataVolume)
				if !ok {
					return false
				}
				newDV, ok := e.ObjectNew.(*cdiv1.DataVolume)
				if !ok {
					return false
				}

				return oldDV.Status.Progress != newDV.Status.Progress
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on DV: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.PersistentVolumeClaim{}),
		handler.EnqueueRequestForOwner(
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&virtv2.VirtualDisk{},
			handler.OnlyControllerOwner(),
		), predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldPVC, ok := e.ObjectOld.(*corev1.PersistentVolumeClaim)
				if !ok {
					return false
				}
				newPVC, ok := e.ObjectNew.(*corev1.PersistentVolumeClaim)
				if !ok {
					return false
				}

				return oldPVC.Status.Capacity[corev1.ResourceStorage] != newPVC.Status.Capacity[corev1.ResourceStorage]
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on PVC: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueDisksAttachedToVM()),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return r.vmHasAttachedDisks(e.Object)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return r.vmHasAttachedDisks(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return r.vmHasAttachedDisks(e.ObjectOld) || r.vmHasAttachedDisks(e.ObjectNew)
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VMs: %w", err)
	}

	return nil
}

func (r *Reconciler) factory() *virtv2.VirtualDisk {
	return &virtv2.VirtualDisk{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualDisk) virtv2.VirtualDiskStatus {
	return obj.Status
}

func (r *Reconciler) enqueueDisksAttachedToVM() handler.MapFunc {
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		vm, ok := obj.(*virtv2.VirtualMachine)
		if !ok {
			return nil
		}

		var requests []reconcile.Request

		for _, bda := range vm.Status.BlockDeviceRefs {
			if bda.Kind != virtv2.DiskDevice {
				continue
			}

			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      bda.Name,
				Namespace: vm.Namespace,
			}})
		}

		return requests
	}
}

func (r *Reconciler) vmHasAttachedDisks(obj client.Object) bool {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return false
	}

	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == virtv2.DiskDevice {
			return true
		}
	}

	return false
}