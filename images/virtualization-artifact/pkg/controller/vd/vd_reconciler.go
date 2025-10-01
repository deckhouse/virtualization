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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
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

		if vd.Changed().Status.Target.PersistentVolumeClaim == "" {
			logger.FromContext(ctx).Error("Target.PersistentVolumeClaim is empty, restore previous value. Please report a bug.")
			vdsupplements.SetPVCName(vd.Changed(), vd.Current().Status.Target.PersistentVolumeClaim)
		}

		return vd.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{},
			&handler.TypedEnqueueRequestForObject[*virtv2.VirtualDisk]{},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
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

	mgrClient := mgr.GetClient()
	for _, w := range []Watcher{
		watcher.NewPersistentVolumeClaimWatcher(mgrClient),
		watcher.NewVirtualDiskSnapshotWatcher(mgrClient),
		watcher.NewStorageClassWatcher(mgrClient),
		watcher.NewDataVolumeWatcher(),
		watcher.NewVirtualMachineWatcher(),
		watcher.NewResourceQuotaWatcher(mgrClient),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
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
