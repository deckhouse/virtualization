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
	"reflect"
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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/watchers"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
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
	log := logger.FromContext(ctx)

	cvi := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := cvi.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if cvi.IsEmpty() {
		return reconcile.Result{}, nil
	}

	log.Debug("Start cvi reconciliation")

	var requeue bool

	var handlerErrs []error

	for _, h := range r.handlers {
		log.Debug("Run handler", logger.SlogHandler(reflect.TypeOf(h).Elem().Name()))

		var res reconcile.Result
		res, err = h.Handle(ctx, cvi.Changed())
		if err != nil {
			log.Error("Failed to handle cvi", logger.SlogErr(err), logger.SlogHandler(reflect.TypeOf(h).Elem().Name()))
			handlerErrs = append(handlerErrs, err)
		}

		// TODO: merger.
		requeue = requeue || res.Requeue
	}

	cvi.Changed().Status.ObservedGeneration = cvi.Changed().Generation

	err = cvi.Update(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = errors.Join(handlerErrs...)
	if err != nil {
		return reconcile.Result{}, err
	}

	if requeue {
		log.Debug("Requeue cvi reconciliation")
		return reconcile.Result{
			RequeueAfter: 2 * time.Second,
		}, nil
	}

	log.Debug("Finished cvi reconciliation")
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

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromVDs),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVD, ok := e.ObjectOld.(*virtv2.VirtualDisk)
				if !ok {
					slog.Default().Error(fmt.Sprintf("expected an old VirtualDisk but got a %T", e.ObjectOld))
					return false
				}

				newVD, ok := e.ObjectNew.(*virtv2.VirtualDisk)
				if !ok {
					slog.Default().Error(fmt.Sprintf("expected a new VirtualDisk but got a %T", e.ObjectNew))
					return false
				}

				oldInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, oldVD.Status.Conditions)
				newInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, newVD.Status.Conditions)

				if oldVD.Status.Phase != newVD.Status.Phase || len(oldVD.Status.AttachedToVirtualMachines) != len(newVD.Status.AttachedToVirtualMachines) || oldInUseCondition != newInUseCondition {
					return true
				}

				return false
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VDs: %w", err)
	}

	cviFromVIEnqueuer := watchers.NewClusterVirtualImageRequestEnqueuer(mgr.GetClient(), &virtv2.VirtualImage{}, virtv2.ClusterVirtualImageObjectRefKindVirtualImage)
	viWatcher := watchers.NewObjectRefWatcher(watchers.NewVirtualImageFilter(), cviFromVIEnqueuer)
	if err := viWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on VIs: %w", err)
	}

	cviFromCVIEnqueuer := watchers.NewClusterVirtualImageRequestEnqueuer(mgr.GetClient(), &virtv2.ClusterVirtualImage{}, virtv2.ClusterVirtualImageObjectRefKindClusterVirtualImage)
	cviWatcher := watchers.NewObjectRefWatcher(watchers.NewClusterVirtualImageFilter(), cviFromCVIEnqueuer)
	if err := cviWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on CVIs: %w", err)
	}

	return nil
}

func (r *Reconciler) enqueueRequestsFromVDs(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var cviList virtv2.ClusterVirtualImageList
	err := r.client.List(ctx, &cviList, &client.ListOptions{})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list cvi: %s", err))
		return
	}

	for _, cvi := range cviList.Items {
		if cvi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || cvi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if cvi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskKind || cvi.Spec.DataSource.ObjectRef.Name != obj.GetName() && cvi.Spec.DataSource.ObjectRef.Namespace != obj.GetNamespace() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: cvi.Name,
			},
		})
	}

	return
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
