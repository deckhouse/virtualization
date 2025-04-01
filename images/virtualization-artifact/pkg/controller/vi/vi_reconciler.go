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

package vi

import (
	"context"
	"fmt"
	"log/slog"

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

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/controller/watchers"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, vi *virtv2.VirtualImage) (reconcile.Result, error)
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

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vi := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vi.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vi.IsEmpty() {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, vi.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vi.Changed().Status.ObservedGeneration = vi.Changed().Generation

		return vi.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{}),
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
		source.Kind(mgr.GetCache(), &cdiv1.DataVolume{}),
		handler.EnqueueRequestForOwner(
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&virtv2.VirtualImage{},
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
			&virtv2.VirtualImage{},
		), predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
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

				return oldPVC.Status.Phase != newPVC.Status.Phase && newPVC.Status.Phase == corev1.ClaimBound
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on PVC: %w", err)
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

	viFromVIEnqueuer := watchers.NewVirtualImageRequestEnqueuer(mgr.GetClient(), &virtv2.VirtualImage{}, virtv2.VirtualImageObjectRefKindVirtualImage)
	viWatcher := watchers.NewObjectRefWatcher(watchers.NewVirtualImageFilter(), viFromVIEnqueuer)
	if err := viWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on VIs: %w", err)
	}

	viFromCVIEnqueuer := watchers.NewVirtualImageRequestEnqueuer(mgr.GetClient(), &virtv2.ClusterVirtualImage{}, virtv2.VirtualImageObjectRefKindClusterVirtualImage)
	cviWatcher := watchers.NewObjectRefWatcher(watchers.NewClusterVirtualImageFilter(), viFromCVIEnqueuer)
	if err := cviWatcher.Run(mgr, ctr); err != nil {
		return fmt.Errorf("error setting watch on CVIs: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewPodWatcher(mgr.GetClient()),
		watcher.NewStorageClassWatcher(mgr.GetClient()),
		watcher.NewVirtualMachineWatcher(mgr.GetClient()),
		watcher.NewVirtualDiskSnapshotWatcher(mgr.GetClient()),
		watcher.NewModuleConfigWatcher(mgr.GetClient()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("error setting watcher: %w", err)
		}
	}

	return nil
}

func (r *Reconciler) enqueueRequestsFromVDs(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	var viList virtv2.VirtualImageList
	err := r.client.List(ctx, &viList, &client.ListOptions{
		Namespace: obj.GetNamespace(),
	})
	if err != nil {
		slog.Default().Error(fmt.Sprintf("failed to list vi: %s", err))
		return
	}

	for _, vi := range viList.Items {
		if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || vi.Spec.DataSource.ObjectRef == nil {
			continue
		}

		if vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskKind || vi.Spec.DataSource.ObjectRef.Name != obj.GetName() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vi.Name,
				Namespace: vi.Namespace,
			},
		})
	}

	return
}

func (r *Reconciler) factory() *virtv2.VirtualImage {
	return &virtv2.VirtualImage{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualImage) virtv2.VirtualImageStatus {
	return obj.Status
}
