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
	"fmt"
	"reflect"

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

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/controller/watchers"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error)
}

type Reconciler struct {
	handlers []Handler
	client   client.Client
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		handlers: handlers,
		client:   client,
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vd := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vd.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vd.IsEmpty() {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, vd.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vd.Changed().Status.ObservedGeneration = vd.Changed().Generation

		return vd.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{}),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				return !reflect.DeepEqual(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers()) || e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
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
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldDV, ok := e.ObjectOld.(*cdiv1.DataVolume)
				if !ok {
					return false
				}
				newDV, ok := e.ObjectNew.(*cdiv1.DataVolume)
				if !ok {
					return false
				}

				if oldDV.Status.Progress != newDV.Status.Progress {
					return true
				}

				if oldDV.Status.Phase != newDV.Status.Phase && newDV.Status.Phase == cdiv1.Succeeded {
					return true
				}

				dvRunning := service.GetDataVolumeCondition(cdiv1.DataVolumeRunning, newDV.Status.Conditions)
				return dvRunning != nil && dvRunning.Reason == "Error"
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on DV: %w", err)
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

	vdFromVIEnqueuer := watchers.NewVirtualDiskRequestEnqueuer(mgr.GetClient(), &virtv2.VirtualImage{}, virtv2.VirtualDiskObjectRefKindVirtualImage)
	viWatcher := watchers.NewObjectRefWatcher(watchers.NewVirtualImageFilter(), vdFromVIEnqueuer)
	if err := viWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on VIs: %w", err)
	}

	vdFromCVIEnqueuer := watchers.NewVirtualDiskRequestEnqueuer(mgr.GetClient(), &virtv2.ClusterVirtualImage{}, virtv2.VirtualDiskObjectRefKindClusterVirtualImage)
	cviWatcher := watchers.NewObjectRefWatcher(watchers.NewClusterVirtualImageFilter(), vdFromCVIEnqueuer)
	if err := cviWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on CVIs: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewPersistentVolumeClaimWatcher(mgr.GetClient()),
		watcher.NewVirtualDiskSnapshotWatcher(mgr.GetClient()),
		watcher.NewStorageClassWatcher(mgr.GetClient()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("error setting watcher: %w", err)
		}
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

		for _, bdr := range vm.Status.BlockDeviceRefs {
			if bdr.Kind != virtv2.DiskDevice {
				continue
			}

			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      bdr.Name,
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
