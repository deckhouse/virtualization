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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/controller/watchers"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
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
	log := logger.FromContext(ctx)

	vd := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vd.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vd.IsEmpty() {
		return reconcile.Result{}, nil
	}

	log.Debug("Start vd reconciliation")

	var result reconcile.Result
	var handlerErrs []error

	for _, h := range r.handlers {
		log.Debug("Run handler", logger.SlogHandler(reflect.TypeOf(h).Elem().Name()))
		var res reconcile.Result
		res, err = h.Handle(ctx, vd.Changed())
		switch {
		case err == nil: // OK.
		case k8serrors.IsConflict(err):
			log.Debug("Failed to handle vd", logger.SlogErr(err), logger.SlogHandler(reflect.TypeOf(h).Elem().Name()))
			result.Requeue = true
		default:
			log.Error("Failed to handle vd", logger.SlogErr(err), logger.SlogHandler(reflect.TypeOf(h).Elem().Name()))
			handlerErrs = append(handlerErrs, err)
		}

		result = service.MergeResults(result, res)
	}

	vd.Changed().Status.ObservedGeneration = vd.Changed().Generation

	err = vd.Update(ctx)
	switch {
	case err == nil: // OK.
	case k8serrors.IsConflict(err):
		log.Debug("Failed to update vi", logger.SlogErr(err))
		result.Requeue = true
	default:
		return reconcile.Result{}, err
	}

	err = errors.Join(handlerErrs...)
	if err != nil {
		return reconcile.Result{}, err
	}

	return result, nil
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
		source.Kind(mgr.GetCache(), &corev1.PersistentVolumeClaim{}),
		handler.EnqueueRequestForOwner(
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&virtv2.VirtualDisk{},
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

				if oldPVC.Status.Capacity[corev1.ResourceStorage] != newPVC.Status.Capacity[corev1.ResourceStorage] {
					return true
				}

				if service.GetPersistentVolumeClaimCondition(corev1.PersistentVolumeClaimResizing, oldPVC.Status.Conditions) != nil ||
					service.GetPersistentVolumeClaimCondition(corev1.PersistentVolumeClaimResizing, newPVC.Status.Conditions) != nil {
					return true
				}

				return oldPVC.Status.Phase != newPVC.Status.Phase
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
