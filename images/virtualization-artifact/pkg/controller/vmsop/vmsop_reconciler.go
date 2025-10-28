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

package vmsop

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Reconciler struct {
	client     client.Client
	controller SubController
}

func NewReconciler(client client.Client, controller SubController) *Reconciler {
	return &Reconciler{
		client:     client,
		controller: controller,
	}
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range r.controller.Watchers() {
		if err := w.Watch(mgr, ctr); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vmsop := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmsop.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmsop.IsEmpty() {
		return reconcile.Result{}, nil
	}

	if !r.controller.ShouldReconcile(vmsop.Changed()) {
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[reconciler.Handler[*v1alpha2.VirtualMachineSnapshotOperation]](r.controller.Handlers())
	rec.SetHandlerExecutor(func(ctx context.Context, h reconciler.Handler[*v1alpha2.VirtualMachineSnapshotOperation]) (reconcile.Result, error) {
		return h.Handle(ctx, vmsop.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vmsop.Changed().Status.ObservedGeneration = vmsop.Changed().Generation

		return vmsop.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *v1alpha2.VirtualMachineSnapshotOperation {
	return &v1alpha2.VirtualMachineSnapshotOperation{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.VirtualMachineSnapshotOperation) v1alpha2.VirtualMachineSnapshotOperationStatus {
	return obj.Status
}
