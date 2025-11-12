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
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (reconcile.Result, error)
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Reconciler struct {
	handlers   []Handler
	client     client.Client
	virtClient kubeclient.Client
}

func NewReconciler(client client.Client, virtClient kubeclient.Client, handlers ...Handler) (*Reconciler, error) {
	return &Reconciler{
		client:     client,
		virtClient: virtClient,
		handlers:   handlers,
	}, nil
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	for _, w := range []Watcher{
		watcher.NewVirtualMachineIPAddressWatcher(),
		watcher.NewVirtualMachineIPAddressLeaseWatcher(mgr.GetClient()),
		watcher.NewVirtualMachineWatcher(mgr.GetClient()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	vmip := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmip.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmip.IsEmpty() {
		return reconcile.Result{}, nil
	}

	log := logger.FromContext(ctx).
		With("staticIP", vmip.Changed().Spec.StaticIP).
		With("addressIP", vmip.Changed().Status.Address)
	ctx = logger.ToContext(ctx, log)

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, vmip.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vmip.Changed().Status.ObservedGeneration = vmip.Changed().Generation

		return vmip.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *v1alpha2.VirtualMachineIPAddress {
	return &v1alpha2.VirtualMachineIPAddress{}
}

func (r *Reconciler) statusGetter(obj *v1alpha2.VirtualMachineIPAddress) v1alpha2.VirtualMachineIPAddressStatus {
	return obj.Status
}
